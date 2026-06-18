package cilium

import "fmt"

type PolicyPresetMeta struct {
	Key         string `json:"key"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Icon        string `json:"icon"`
	Ports       string `json:"ports,omitempty"`
	Applied     bool   `json:"applied"`
}

func hostPolicy(name, desc string, ports []struct{ port, proto string }) string {
	var portBlocks string
	for _, p := range ports {
		portBlocks += fmt.Sprintf("      - port: \"%s\"\n        protocol: %s\n", p.port, p.proto)
	}
	return fmt.Sprintf(`apiVersion: cilium.io/v2
kind: CiliumClusterwideNetworkPolicy
metadata:
  name: %s
spec:
  description: "%s"
  nodeSelector:
    matchLabels:
      kubernetes.io/os: linux
  ingress:
  - toPorts:
    - ports:
%s`, name, desc, portBlocks)
}

func presetYAML(key, panelPort string) (string, string, error) {
	switch key {
	case "ssh":
		return "owpanel-host-ssh", hostPolicy("owpanel-host-ssh", "Allow SSH", []struct{ port, proto string }{{"22", "TCP"}}), nil
	case "http":
		return "owpanel-host-http", hostPolicy("owpanel-host-http", "Allow HTTP", []struct{ port, proto string }{{"80", "TCP"}}), nil
	case "https":
		return "owpanel-host-https", hostPolicy("owpanel-host-https", "Allow HTTPS", []struct{ port, proto string }{{"443", "TCP"}}), nil
	case "web":
		return "owpanel-host-web", hostPolicy("owpanel-host-web", "Allow HTTP/HTTPS", []struct{ port, proto string }{{"80", "TCP"}, {"443", "TCP"}}), nil
	case "panel":
		if panelPort == "" {
			panelPort = "8888"
		}
		name := "owpanel-host-panel"
		return name, hostPolicy(name, "Allow OWPanel port", []struct{ port, proto string }{{panelPort, "TCP"}}), nil
	case "kube-api":
		return "owpanel-host-kube-api", hostPolicy("owpanel-host-kube-api", "Allow K3s API", []struct{ port, proto string }{{"6443", "TCP"}}), nil
	case "baseline":
		return "", "", fmt.Errorf("baseline 请使用 ApplyBaselinePresets")
	default:
		return "", "", fmt.Errorf("未知预设: %s", key)
	}
}

func ListPresetMeta(panelPort string) []PolicyPresetMeta {
	if panelPort == "" {
		panelPort = "8888"
	}
	return []PolicyPresetMeta{
		{Key: "ssh", Name: "SSH 远程", Description: "放行 TCP 22，避免 Host Firewall 锁死服务器", Icon: "terminal", Ports: "22/tcp"},
		{Key: "web", Name: "网站 HTTP/HTTPS", Description: "放行 80、443，适用于 Nginx/OpenResty 站点", Icon: "globe", Ports: "80,443/tcp"},
		{Key: "panel", Name: "OWPanel 面板", Description: "放行面板端口，便于远程管理", Icon: "monitor", Ports: panelPort + "/tcp"},
		{Key: "kube-api", Name: "K3s API", Description: "放行 6443，集群 kubectl/Helm 通信", Icon: "connection", Ports: "6443/tcp"},
		{Key: "http", Name: "仅 HTTP", Description: "仅放行 80", Icon: "link", Ports: "80/tcp"},
		{Key: "https", Name: "仅 HTTPS", Description: "仅放行 443", Icon: "lock", Ports: "443/tcp"},
	}
}

func (s *Service) ApplyPreset(key, panelPort string) (string, error) {
	_, yaml, err := presetYAML(key, panelPort)
	if err != nil {
		return "", err
	}
	return s.ApplyPolicyYAML(yaml)
}

func (s *Service) ApplyBaselinePresets(panelPort string) ([]string, error) {
	var msgs []string
	for _, key := range []string{"ssh", "web", "panel"} {
		msg, err := s.ApplyPreset(key, panelPort)
		if err != nil {
			return msgs, fmt.Errorf("预设 %s: %w", key, err)
		}
		msgs = append(msgs, msg)
	}
	return msgs, nil
}

func (s *Service) presetApplied(name string, policies []PolicyItem) bool {
	for _, p := range policies {
		if p.Name == name {
			return true
		}
	}
	return false
}

func (s *Service) PresetsWithStatus(panelPort string) ([]PolicyPresetMeta, error) {
	list, err := s.ListPolicies()
	if err != nil {
		list = nil
	}
	out := ListPresetMeta(panelPort)
	for i := range out {
		n, _, _ := presetYAML(out[i].Key, panelPort)
		out[i].Applied = s.presetApplied(n, list)
	}
	return out, nil
}
