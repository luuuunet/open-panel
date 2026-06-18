package cluster

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/luuuunet/owpanel/internal/models"
	"golang.org/x/crypto/ssh"
)

type NodeMonitor struct {
	CPU        float64 `json:"cpu"`
	Memory     float64 `json:"memory"`
	Disk       float64 `json:"disk"`
	Load1      float64 `json:"load1"`
	Hostname   string  `json:"hostname"`
	Uptime     string  `json:"uptime"`
	Collected  string  `json:"collected_at"`
	ViaSSH     bool    `json:"via_ssh"`
}

type SSHTestResult struct {
	OK      bool   `json:"ok"`
	Message string `json:"message"`
	Output  string `json:"output,omitempty"`
}

type ProvisionResult struct {
	Status  string `json:"status"`
	Log     string `json:"log"`
	Message string `json:"message"`
}

func (s *Service) sshHost(node *models.ClusterNode) string {
	if strings.TrimSpace(node.SSHHost) != "" {
		return strings.TrimSpace(node.SSHHost)
	}
	return strings.TrimSpace(node.Host)
}

func (s *Service) sshClient(node *models.ClusterNode) (*ssh.Client, error) {
	host := s.sshHost(node)
	port := node.SSHPort
	if port <= 0 {
		port = 22
	}
	user := strings.TrimSpace(node.SSHUser)
	if user == "" {
		user = "root"
	}
	if strings.TrimSpace(node.SSHPassword) == "" {
		return nil, fmt.Errorf("SSH 密码未配置")
	}
	cfg := &ssh.ClientConfig{
		User:            user,
		Auth:            []ssh.AuthMethod{ssh.Password(node.SSHPassword)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         12 * time.Second,
	}
	return ssh.Dial("tcp", fmt.Sprintf("%s:%d", host, port), cfg)
}

func sshRun(client *ssh.Client, cmd string) (string, error) {
	session, err := client.NewSession()
	if err != nil {
		return "", err
	}
	defer session.Close()
	var buf bytes.Buffer
	session.Stdout = &buf
	session.Stderr = &buf
	if err := session.Run(cmd); err != nil {
		return buf.String(), fmt.Errorf("%w: %s", err, strings.TrimSpace(buf.String()))
	}
	return buf.String(), nil
}

func (s *Service) TestSSH(id uint) (*SSHTestResult, error) {
	node, err := s.GetNode(id)
	if err != nil {
		return nil, err
	}
	if node.IsLocal {
		return &SSHTestResult{OK: true, Message: "本机节点无需 SSH"}, nil
	}
	client, err := s.sshClient(node)
	if err != nil {
		return &SSHTestResult{OK: false, Message: err.Error()}, nil
	}
	defer client.Close()
	out, err := sshRun(client, "uname -a && hostname")
	if err != nil {
		return &SSHTestResult{OK: false, Message: err.Error(), Output: out}, nil
	}
	return &SSHTestResult{OK: true, Message: "SSH 连接成功", Output: strings.TrimSpace(out)}, nil
}

func (s *Service) CollectMonitor(id uint) (*NodeMonitor, error) {
	node, err := s.GetNode(id)
	if err != nil {
		return nil, err
	}
	if node.IsLocal {
		st, _ := s.dashboard.GetStats()
		m := &NodeMonitor{ViaSSH: false, Collected: time.Now().Format(time.RFC3339)}
		if st != nil {
			m.CPU = st.CPU.UsagePercent
			m.Memory = st.Memory.UsedPercent
			m.Hostname = st.System.Hostname
			m.Load1 = st.Load.Load1
			if len(st.Disk) > 0 {
				m.Disk = st.Disk[0].UsedPercent
			}
		}
		return m, nil
	}
	client, err := s.sshClient(node)
	if err != nil {
		return nil, err
	}
	defer client.Close()
	script := `echo HOST:$(hostname)
echo LOAD:$(awk '{print $1}' /proc/loadavg 2>/dev/null || echo 0)
echo CPU:$(awk '/^cpu /{u=$2+$3+$4; t=0; for(i=2;i<=NF;i++) t+=$i; if(t>0) printf "%.1f", u/t*100; else print 0}' /proc/stat 2>/dev/null || echo 0)
echo MEM:$(free 2>/dev/null | awk '/Mem:/ {if($2>0) printf "%.1f", $3/$2*100; else print 0}')
echo DISK:$(df -P / 2>/dev/null | awk 'NR==2 {gsub(/%/,"",$5); print $5}')`
	out, err := sshRun(client, script)
	if err != nil {
		return nil, err
	}
	m := &NodeMonitor{ViaSSH: true, Collected: time.Now().Format(time.RFC3339)}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if !strings.Contains(line, ":") {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		key, val := parts[0], strings.TrimSpace(parts[1])
		f, _ := strconv.ParseFloat(strings.TrimSuffix(val, "%"), 64)
		switch key {
		case "HOST":
			m.Hostname = val
		case "LOAD":
			m.Load1 = f
		case "CPU":
			m.CPU = f
		case "MEM":
			m.Memory = f
		case "DISK":
			m.Disk = f
		}
	}
	now := time.Now()
	updates := map[string]interface{}{
		"hostname": m.Hostname, "cpu_percent": m.CPU, "mem_percent": m.Memory,
		"disk_percent": m.Disk, "load1": m.Load1, "last_seen_at": now,
	}
	if m.CPU > 0 || m.Memory > 0 {
		updates["status"] = "online"
	}
	s.db.Model(node).Updates(updates)
	return m, nil
}

func (s *Service) syncNodeViaSSH(node *models.ClusterNode) error {
	m, err := s.CollectMonitor(node.ID)
	if err != nil {
		return err
	}
	if m.CPU > 0 || m.Memory > 0 {
		return nil
	}
	return fmt.Errorf("ssh metrics empty")
}

func (s *Service) ProvisionNode(id uint) (*ProvisionResult, error) {
	node, err := s.GetNode(id)
	if err != nil {
		return nil, err
	}
	if node.IsLocal {
		return nil, fmt.Errorf("本机节点无需远程搭建")
	}
	client, err := s.sshClient(node)
	if err != nil {
		return nil, err
	}
	defer client.Close()

	s.db.Model(node).Updates(map[string]interface{}{
		"provision_status": "provisioning", "provision_log": "开始自动搭建…",
	})

	role := strings.TrimSpace(node.ProvisionRole)
	if role == "" {
		role = "lb_backend"
	}
	port := node.WebsitePort
	if port <= 0 {
		port = 80
	}

	var log strings.Builder
	log.WriteString(fmt.Sprintf("角色: %s, 后端端口: %d\n", role, port))

	preflight := `set -e
export DEBIAN_FRONTEND=noninteractive
if command -v apt-get >/dev/null 2>&1; then PKG=apt; elif command -v yum >/dev/null 2>&1; then PKG=yum; else PKG=unknown; fi
echo PKG:$PKG`
	out, err := sshRun(client, preflight)
	log.WriteString(out)
	if err != nil {
		s.finishProvision(node, "failed", log.String(), err.Error())
		return &ProvisionResult{Status: "failed", Log: log.String(), Message: err.Error()}, err
	}

	switch role {
	case "db_slave", "db_master":
		err = s.provisionMySQL(client, role, &log)
	case "worker":
		err = s.provisionWorker(client, port, &log)
	default:
		err = s.provisionLBBackend(client, port, &log)
	}

	if err != nil {
		s.finishProvision(node, "failed", log.String(), err.Error())
		return &ProvisionResult{Status: "failed", Log: log.String(), Message: err.Error()}, err
	}

	host := s.sshHost(node)
	if node.WebsiteHost == "" {
		s.db.Model(node).Update("website_host", host)
	}
	s.finishProvision(node, "ready", log.String(), "自动搭建完成")
	_ = s.SyncNode(node.ID)
	return &ProvisionResult{Status: "ready", Log: log.String(), Message: "负载节点已就绪，可作为 LB 后端"}, nil
}

func (s *Service) finishProvision(node *models.ClusterNode, status, logText, msg string) {
	s.db.Model(node).Updates(map[string]interface{}{
		"provision_status": status,
		"provision_log":    logText,
	})
}

func (s *Service) provisionLBBackend(client *ssh.Client, port int, log *strings.Builder) error {
	script := fmt.Sprintf(`set -e
if command -v nginx >/dev/null 2>&1; then echo nginx:ok
elif command -v apt-get >/dev/null 2>&1; then apt-get update -qq && apt-get install -y nginx
elif command -v yum >/dev/null 2>&1; then yum install -y nginx
else echo "请手动安装 nginx"; exit 1; fi
mkdir -p /var/www/owpanel-backend
echo 'OWPanel LB Backend OK' > /var/www/owpanel-backend/index.html
CONF=/etc/nginx/conf.d/owpanel-backend.conf
cat > "$CONF" <<'NGX'
server {
    listen %d;
    server_name _;
    root /var/www/owpanel-backend;
    index index.html;
    location / { try_files $uri $uri/ =404; }
    location /health { return 200 'ok'; add_header Content-Type text/plain; }
}
NGX
nginx -t && (systemctl reload nginx 2>/dev/null || systemctl restart nginx 2>/dev/null || service nginx reload 2>/dev/null || true)
echo done`, port)
	out, err := sshRun(client, script)
	log.WriteString(out)
	return err
}

func (s *Service) provisionWorker(client *ssh.Client, port int, log *strings.Builder) error {
	err := s.provisionLBBackend(client, port, log)
	if err != nil {
		return err
	}
	out, e2 := sshRun(client, "mkdir -p /opt/owpanel && echo 'worker ready' > /opt/owpanel/.cluster-worker")
	log.WriteString(out)
	return e2
}

func (s *Service) provisionMySQL(client *ssh.Client, role string, log *strings.Builder) error {
	script := `set -e
export DEBIAN_FRONTEND=noninteractive
if command -v mysql >/dev/null 2>&1; then echo mysql:installed
elif command -v apt-get >/dev/null 2>&1; then
  debconf-set-selections <<< 'mysql-server mysql-server/root_password password temp_root_pass' || true
  debconf-set-selections <<< 'mysql-server mysql-server/root_password_again password temp_root_pass' || true
  apt-get update -qq && apt-get install -y mysql-server
elif command -v yum >/dev/null 2>&1; then yum install -y mysql-server || yum install -y mariadb-server
else echo "请手动安装 MySQL/MariaDB"; exit 1; fi
systemctl enable mysqld 2>/dev/null || systemctl enable mysql 2>/dev/null || systemctl enable mariadb 2>/dev/null || true
systemctl start mysqld 2>/dev/null || systemctl start mysql 2>/dev/null || systemctl start mariadb 2>/dev/null || true
echo mysql:ready`
	out, err := sshRun(client, script)
	log.WriteString(out)
	if err != nil {
		return err
	}
	if role == "db_master" {
		out2, err2 := sshRun(client, "mysql -e \"SHOW VARIABLES LIKE 'server_id';\" 2>/dev/null || true")
		log.WriteString(out2)
		return err2
	}
	return nil
}

func nodeHasSSH(node *models.ClusterNode) bool {
	return !node.IsLocal && strings.TrimSpace(node.SSHPassword) != ""
}
