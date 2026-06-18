package cilium

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type PolicyItem struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Kind      string `json:"kind"`
	Created   string `json:"created,omitempty"`
}

func (s *Service) ListPolicies() ([]PolicyItem, error) {
	var items []PolicyItem
	for _, spec := range []struct {
		kind  string
		scope string
	}{
		{"ciliumclusterwidenetworkpolicies", "cluster"},
		{"ciliumnetworkpolicies", "namespaced"},
	} {
		out, err := kubectl("get", spec.kind, "-A", "-o", "custom-columns=NAME:.metadata.name,NAMESPACE:.metadata.namespace,AGE:.metadata.creationTimestamp", "--no-headers")
		if err != nil {
			if strings.Contains(err.Error(), "NotFound") || strings.Contains(err.Error(), "not found") {
				continue
			}
			return nil, err
		}
		for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			fields := strings.Fields(line)
			if len(fields) < 2 {
				continue
			}
			name := fields[0]
			ns := fields[1]
			kind := "CiliumClusterwideNetworkPolicy"
			if spec.scope == "namespaced" {
				kind = "CiliumNetworkPolicy"
			}
			created := ""
			if len(fields) >= 3 {
				created = fields[2]
			}
			items = append(items, PolicyItem{Name: name, Namespace: ns, Kind: kind, Created: created})
		}
	}
	return items, nil
}

func (s *Service) ApplyPolicyYAML(yaml string) (string, error) {
	yaml = strings.TrimSpace(yaml)
	if yaml == "" {
		return "", fmt.Errorf("策略 YAML 不能为空")
	}
	dir := filepath.Join(os.TempDir(), "owpanel-cilium")
	_ = os.MkdirAll(dir, 0755)
	path := filepath.Join(dir, fmt.Sprintf("policy-%d.yaml", time.Now().UnixNano()))
	if err := os.WriteFile(path, []byte(yaml), 0600); err != nil {
		return "", err
	}
	defer os.Remove(path)
	out, err := kubectl("apply", "-f", path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

func (s *Service) DeletePolicy(kind, namespace, name string) error {
	kind = strings.TrimSpace(kind)
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("缺少策略名称")
	}
	args := []string{"delete"}
	switch strings.ToLower(kind) {
	case "", "ciliumclusterwidenetworkpolicy", "ccnp":
		args = append(args, "ciliumclusterwidenetworkpolicy", name)
	case "ciliumnetworkpolicy", "cnp":
		ns := strings.TrimSpace(namespace)
		if ns == "" || ns == "cluster" {
			return fmt.Errorf("命名空间策略需要 namespace")
		}
		args = append(args, "-n", ns, "ciliumnetworkpolicy", name)
	default:
		return fmt.Errorf("不支持的策略类型: %s", kind)
	}
	_, err := kubectl(args...)
	return err
}

func DefaultHostSSHPolicy() string {
	return `apiVersion: cilium.io/v2
kind: CiliumClusterwideNetworkPolicy
metadata:
  name: owpanel-host-ssh
spec:
  description: "Allow SSH to node (Host Firewall)"
  nodeSelector:
    matchLabels:
      kubernetes.io/os: linux
  ingress:
  - toPorts:
    - ports:
      - port: "22"
        protocol: TCP
`
}
