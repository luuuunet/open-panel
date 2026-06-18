package cilium

import "fmt"

type SetupStep struct {
	Key         string `json:"key"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Done        bool   `json:"done"`
	Current     bool   `json:"current"`
	Action      string `json:"action,omitempty"`
}

type ChecklistItem struct {
	Key    string `json:"key"`
	Label  string `json:"label"`
	Pass   bool   `json:"pass"`
	Level  string `json:"level"`
	Hint   string `json:"hint,omitempty"`
}

type DashboardResult struct {
	Status      *StatusResult     `json:"status"`
	HealthScore int               `json:"health_score"`
	SetupSteps  []SetupStep       `json:"setup_steps"`
	Checklist   []ChecklistItem   `json:"checklist"`
	PolicyCount int               `json:"policy_count"`
	Presets     []PolicyPresetMeta `json:"presets"`
}

func (s *Service) Dashboard(panelPort string) (*DashboardResult, error) {
	st, err := s.Status()
	if err != nil {
		return nil, err
	}
	cfg, _ := s.GetConfig()
	policies, _ := s.ListPolicies()
	presets, _ := s.PresetsWithStatus(panelPort)

	hasSSH := s.presetApplied("owpanel-host-ssh", policies)
	hasWeb := s.presetApplied("owpanel-host-web", policies)

	steps := []SetupStep{
		{
			Key: "k3s", Title: "安装 K3s", Description: "轻量 Kubernetes 运行时",
			Done: st.K3sRunning, Current: !st.K3sRunning, Action: "install_k3s",
		},
		{
			Key: "cilium", Title: "安装 Cilium", Description: "eBPF CNI 与 Host Firewall",
			Done: st.CiliumReady, Current: st.K3sRunning && !st.CiliumReady, Action: "install_cilium",
		},
		{
			Key: "host_fw", Title: "启用 Host Firewall", Description: "Helm 应用 eBPF 节点防火墙",
			Done: st.CiliumReady && cfg != nil && cfg.HostFirewallEnabled,
			Current: st.CiliumReady && (cfg == nil || !cfg.HostFirewallEnabled), Action: "apply_helm",
		},
		{
			Key: "baseline", Title: "应用基础策略", Description: "SSH + Web + 面板端口",
			Done: hasSSH && hasWeb, Current: st.CiliumReady && (!hasSSH || !hasWeb), Action: "apply_baseline",
		},
		{
			Key: "audit", Title: "关闭审计模式", Description: "确认无误后正式拦截",
			Done: cfg != nil && !cfg.AuditMode && st.CiliumReady,
			Current: cfg != nil && cfg.AuditMode && hasSSH, Action: "disable_audit",
		},
	}

	checklist := []ChecklistItem{
		{Key: "linux", Label: "Linux 服务器", Pass: !st.LinuxOnly, Level: "high", Hint: "Cilium 需 Linux"},
		{Key: "kernel", Label: "内核 5.10+", Pass: st.KernelOK, Level: "medium", Hint: "升级内核获得完整 eBPF 能力"},
		{Key: "k3s", Label: "K3s 运行中", Pass: st.K3sRunning, Level: "high"},
		{Key: "cilium", Label: "Cilium 就绪", Pass: st.CiliumReady, Level: "high"},
		{Key: "host_fw", Label: "Host Firewall", Pass: cfg != nil && cfg.HostFirewallEnabled, Level: "high"},
		{Key: "ssh_policy", Label: "SSH 策略", Pass: hasSSH, Level: "high", Hint: "避免锁死 SSH"},
		{Key: "audit", Label: "审计模式（建议先开）", Pass: cfg != nil && cfg.AuditMode, Level: "low", Hint: "上线前可关闭"},
	}

	score := 0
	if !st.LinuxOnly {
		score += 10
	}
	if st.KernelOK {
		score += 10
	}
	if st.K3sRunning {
		score += 20
	}
	if st.CiliumReady {
		score += 25
	}
	if cfg != nil && cfg.HostFirewallEnabled {
		score += 15
	}
	if hasSSH {
		score += 10
	}
	if hasWeb {
		score += 5
	}
	if cfg != nil && !cfg.AuditMode && st.CiliumReady {
		score += 5
	}

	return &DashboardResult{
		Status:      st,
		HealthScore: score,
		SetupSteps:  steps,
		Checklist:   checklist,
		PolicyCount: len(policies),
		Presets:     presets,
	}, nil
}

type WizardResult struct {
	Message  string   `json:"message"`
	Steps    []string `json:"steps"`
	Install  *InstallStackResult `json:"install,omitempty"`
}

func (s *Service) RunWizard(panelPort string) (*WizardResult, error) {
	if !s.linuxHost() {
		return nil, fmt.Errorf("Cilium 仅支持 Linux 服务器")
	}
	res := &WizardResult{Steps: []string{}}
	cfg, err := s.GetConfig()
	if err != nil {
		return nil, err
	}
	cfg.HostFirewallEnabled = true
	cfg.HubbleEnabled = true
	cfg.HubbleUIEnabled = true
	cfg.AuditMode = true
	if _, err := s.UpdateConfig(cfg); err != nil {
		return nil, err
	}

	if !s.k3sRunning() || !s.appInstalled(ciliumAppKey) {
		inst, err := s.InstallStack(true, true)
		if err != nil {
			return nil, err
		}
		res.Install = inst
		res.Steps = append(res.Steps, "已安装 K3s + Cilium")
	} else {
		res.Steps = append(res.Steps, "K3s + Cilium 已就绪")
	}

	if _, err := s.ApplyHostFirewall(); err != nil {
		return nil, fmt.Errorf("应用 Helm: %w", err)
	}
	res.Steps = append(res.Steps, "已应用 Host Firewall / Hubble 配置")

	msgs, err := s.ApplyBaselinePresets(panelPort)
	if err != nil {
		return nil, fmt.Errorf("基础策略: %w", err)
	}
	res.Steps = append(res.Steps, "已应用 SSH + Web + 面板策略")
	_ = msgs

	res.Message = "向导完成：请确认 Hubble/日志无异常后，在设置中关闭审计模式"
	return res, nil
}

func (s *Service) SetAuditMode(enabled bool) (*CiliumConfig, error) {
	cfg, err := s.GetConfig()
	if err != nil {
		return nil, err
	}
	cfg.AuditMode = enabled
	if _, err := s.UpdateConfig(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}
