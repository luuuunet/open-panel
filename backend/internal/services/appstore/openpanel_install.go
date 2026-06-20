package appstore

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/luuuunet/owpanel/internal/secrets"
	"github.com/luuuunet/owpanel/internal/services/settings"
)

const openpanelAppKey = "openpanel-analytics"

func tryOpenpanelInstall(key, version, installPath, dataDir string) (bool, error) {
	if key != openpanelAppKey {
		return false, nil
	}
	_ = version
	_ = installPath
	return true, installOpenpanelAnalytics(dataDir)
}

func tryOpenpanelUninstall(key, dataDir string) (bool, error) {
	if key != openpanelAppKey {
		return false, nil
	}
	return true, uninstallOpenpanelAnalytics(dataDir)
}

func tryOpenpanelServiceAction(key, action, dataDir string) (bool, error) {
	if key != openpanelAppKey {
		return false, nil
	}
	dir := openpanelAppDir(dataDir)
	cf := filepath.Join(dir, "docker-compose.yml")
	if _, err := os.Stat(cf); err != nil {
		return true, fmt.Errorf("A/B 测试服务尚未安装")
	}
	if _, err := exec.LookPath("docker"); err != nil {
		return true, fmt.Errorf("docker 不可用")
	}
	switch action {
	case "start":
		return true, runDockerComposeInDir(dir, "up", "-d")
	case "stop":
		return true, runDockerComposeInDir(dir, "stop")
	case "restart":
		if err := runDockerComposeInDir(dir, "stop"); err != nil {
			return true, err
		}
		return true, runDockerComposeInDir(dir, "up", "-d")
	default:
		return true, nil
	}
}

func openpanelAppDir(dataDir string) string {
	return settings.DockerAppPath(dataDir, openpanelAppKey)
}

func OpenpanelComposeStatus(dataDir string) string {
	dir := openpanelAppDir(dataDir)
	cf := filepath.Join(dir, "docker-compose.yml")
	if _, err := os.Stat(cf); err != nil {
		return "stopped"
	}
	if _, err := exec.LookPath("docker"); err != nil {
		return "unknown"
	}
	if dockerComposePSRunning(dir) {
		return "running"
	}
	return "stopped"
}

func OpenpanelInstalled(dataDir string) bool {
	cf := filepath.Join(openpanelAppDir(dataDir), "docker-compose.yml")
	_, err := os.Stat(cf)
	return err == nil
}

// appInstalledOnDisk reports install artifacts present even when the apps table was not updated
// (e.g. compose up failed after writing files, or manual repair).
func appInstalledOnDisk(key, dataDir string) bool {
	if key == openpanelAppKey {
		return OpenpanelInstalled(dataDir)
	}
	return false
}

func installOpenpanelAnalytics(dataDir string) error {
	if err := ensureDockerEngine(dataDir); err != nil {
		return err
	}
	dir := openpanelAppDir(dataDir)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create app dir: %w", err)
	}

	pgPass, err := secrets.GeneratePassword(24)
	if err != nil {
		pgPass = "openpanel_changeme"
	}
	cookieSecret, err := secrets.GeneratePassword(32)
	if err != nil {
		cookieSecret = pgPass
	}

	composePath := filepath.Join(dir, "docker-compose.yml")
	envPath := filepath.Join(dir, ".env")
	chDir := filepath.Join(dir, "clickhouse")
	if err := os.MkdirAll(chDir, 0755); err != nil {
		return fmt.Errorf("create clickhouse config dir: %w", err)
	}
	for name, content := range map[string]string{
		"clickhouse-config.xml":      openpanelClickhouseConfigXML,
		"clickhouse-user-config.xml": openpanelClickhouseUserConfigXML,
		"init-db.sh":                 openpanelClickhouseInitDB,
	} {
		p := filepath.Join(chDir, name)
		mode := os.FileMode(0644)
		if name == "init-db.sh" {
			mode = 0755
		}
		if err := os.WriteFile(p, []byte(content), mode); err != nil {
			return fmt.Errorf("write %s: %w", name, err)
		}
	}
	if err := os.WriteFile(composePath, []byte(openpanelComposeFile()), 0644); err != nil {
		return fmt.Errorf("write docker-compose.yml: %w", err)
	}
	if err := os.WriteFile(envPath, []byte(openpanelEnvFile(pgPass, cookieSecret)), 0600); err != nil {
		return fmt.Errorf("write .env: %w", err)
	}
	logInstallLine(fmt.Sprintf("网站产品分析配置已写入 %s", dir))

	_ = runDockerComposeInDir(dir, "down", "--remove-orphans")
	if err := runDockerComposeInDir(dir, "pull"); err != nil {
		logInstallLine("docker compose pull 警告: " + err.Error())
	}
	if err := runDockerComposeInDir(dir, "up", "-d"); err != nil {
		return fmt.Errorf("docker compose up: %w", err)
	}
	logInstallLine("网站产品分析已启动（仪表盘 :3300，API :3333）")
	return nil
}

func uninstallOpenpanelAnalytics(dataDir string) error {
	dir := openpanelAppDir(dataDir)
	cf := filepath.Join(dir, "docker-compose.yml")
	if _, err := os.Stat(cf); err == nil {
		if _, lookErr := exec.LookPath("docker"); lookErr == nil {
			_ = runDockerComposeInDir(dir, "down")
		}
	}
	return os.RemoveAll(dir)
}

func openpanelComposeFile() string {
	return `services:
  op-db:
    image: postgres:14-alpine
    restart: unless-stopped
    volumes:
      - op-db-data:/var/lib/postgresql/data
    environment:
      POSTGRES_USER: postgres
      POSTGRES_PASSWORD: ${POSTGRES_PASSWORD}
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U postgres"]
      interval: 10s
      timeout: 5s
      retries: 5

  op-kv:
    image: redis:7.2.5-alpine
    restart: unless-stopped
    volumes:
      - op-kv-data:/data
    command: ["redis-server", "--maxmemory-policy", "noeviction"]
    healthcheck:
      test: ["CMD-SHELL", "redis-cli ping"]
      interval: 10s
      timeout: 5s
      retries: 5

  op-ch:
    image: clickhouse/clickhouse-server:25.10.2.65
    restart: unless-stopped
    volumes:
      - op-ch-data:/var/lib/clickhouse
      - op-ch-logs:/var/log/clickhouse-server
      - ./clickhouse/clickhouse-config.xml:/etc/clickhouse-server/config.d/op-config.xml:ro
      - ./clickhouse/clickhouse-user-config.xml:/etc/clickhouse-server/users.d/op-user-config.xml:ro
      - ./clickhouse/init-db.sh:/docker-entrypoint-initdb.d/init-db.sh:ro
    environment:
      CLICKHOUSE_SKIP_USER_SETUP: 1
    ulimits:
      nofile:
        soft: 262144
        hard: 262144
    healthcheck:
      test: ["CMD-SHELL", "clickhouse-client --query 'SELECT 1' || exit 1"]
      interval: 10s
      timeout: 5s
      retries: 5

  op-api:
    image: lindesvard/openpanel-api:2
    restart: unless-stopped
    ports:
      - "3333:3000"
    command: >
      sh -c "
      echo 'Running migrations...';
      CI=true pnpm -r run migrate:deploy;
      pnpm start
      "
    depends_on:
      op-db:
        condition: service_healthy
      op-ch:
        condition: service_healthy
      op-kv:
        condition: service_healthy
    env_file:
      - .env
    healthcheck:
      test: ["CMD-SHELL", "curl -f http://localhost:3000/healthcheck || exit 1"]
      interval: 15s
      timeout: 5s
      retries: 8

  op-dashboard:
    image: lindesvard/openpanel-dashboard:2
    restart: unless-stopped
    ports:
      - "3300:3000"
    depends_on:
      op-api:
        condition: service_healthy
    env_file:
      - .env
    healthcheck:
      test: ["CMD-SHELL", "curl -f http://localhost:3000/api/healthcheck || exit 1"]
      interval: 15s
      timeout: 5s
      retries: 8

  op-worker:
    image: lindesvard/openpanel-worker:2
    restart: unless-stopped
    depends_on:
      op-api:
        condition: service_healthy
    env_file:
      - .env
    healthcheck:
      test: ["CMD-SHELL", "curl -f http://localhost:3000/healthcheck || exit 1"]
      interval: 15s
      timeout: 5s
      retries: 8

volumes:
  op-db-data:
  op-kv-data:
  op-ch-data:
  op-ch-logs:
`
}

func openpanelEnvFile(postgresPassword, cookieSecret string) string {
	return fmt.Sprintf(`NODE_ENV=production
SELF_HOSTED=true
BATCH_SIZE=5000
BATCH_INTERVAL=10000
ALLOW_REGISTRATION=false
ALLOW_INVITATION=true
POSTGRES_PASSWORD=%s
REDIS_URL=redis://op-kv:6379
CLICKHOUSE_URL=http://op-ch:8123/openpanel
DATABASE_URL=postgresql://postgres:%s@op-db:5432/postgres?schema=public
DATABASE_URL_DIRECT=postgresql://postgres:%s@op-db:5432/postgres?schema=public
DASHBOARD_URL=http://localhost:3300
API_URL=http://localhost:3333
COOKIE_SECRET=%s
EMAIL_SENDER=
RESEND_API_KEY=
OP_WORKER_REPLICAS=1
`, postgresPassword, postgresPassword, postgresPassword, cookieSecret)
}
