package k8s

type SetupStep struct {
	Key         string `json:"key"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Done        bool   `json:"done"`
	Current     bool   `json:"current"`
	Action      string `json:"action,omitempty"`
}

type ChecklistItem struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Pass  bool   `json:"pass"`
	Level string `json:"level"`
	Hint  string `json:"hint,omitempty"`
}

type DashboardResult struct {
	Status      *StatusResult   `json:"status"`
	HealthScore int             `json:"health_score"`
	SetupSteps  []SetupStep     `json:"setup_steps"`
	Checklist   []ChecklistItem `json:"checklist"`
}

func (s *Service) Dashboard() (*DashboardResult, error) {
	st, err := s.Status()
	if err != nil {
		return nil, err
	}

	sampleDeployed := s.sampleAppDeployed()

	steps := []SetupStep{
		{
			Key: "k3s", Title: "安装 K3s", Description: "轻量 Kubernetes 控制平面",
			Done: st.K3sRunning, Current: !st.K3sRunning, Action: "install_k3s",
		},
		{
			Key: "verify", Title: "验证集群", Description: "节点与系统 Pod 就绪",
			Done: st.K8sReady, Current: st.K3sRunning && !st.K8sReady, Action: "refresh",
		},
		{
			Key: "sample", Title: "示例应用（可选）", Description: "部署 nginx 验证工作负载",
			Done: sampleDeployed, Current: st.K8sReady && !sampleDeployed, Action: "deploy_sample",
		},
	}

	checklist := []ChecklistItem{
		{Key: "linux", Label: "Linux 服务器", Pass: !st.LinuxOnly, Level: "high", Hint: "K3s 需 Linux"},
		{Key: "k3s", Label: "K3s 运行中", Pass: st.K3sRunning, Level: "high"},
		{Key: "nodes", Label: "节点 Ready", Pass: st.NodesTotal > 0 && st.NodesReady >= st.NodesTotal, Level: "high"},
		{Key: "system", Label: "系统 Pod 健康", Pass: st.SystemPodsTotal > 0 && st.SystemPodsReady >= st.SystemPodsTotal, Level: "high"},
		{Key: "sample", Label: "示例 nginx（可选）", Pass: sampleDeployed, Level: "low"},
	}

	score := 0
	if !st.LinuxOnly {
		score += 15
	}
	if st.K3sRunning {
		score += 30
	}
	if st.NodesTotal > 0 && st.NodesReady >= st.NodesTotal {
		score += 25
	}
	if st.SystemPodsTotal > 0 && st.SystemPodsReady >= st.SystemPodsTotal {
		score += 20
	}
	if sampleDeployed {
		score += 10
	}

	return &DashboardResult{
		Status:      st,
		HealthScore: score,
		SetupSteps:  steps,
		Checklist:   checklist,
	}, nil
}

func (s *Service) sampleAppDeployed() bool {
	out, err := kubectl("get", "deployment", "owpanel-nginx-demo", "-o", "name", "--ignore-not-found")
	if err != nil {
		return false
	}
	return len(out) > 0
}
