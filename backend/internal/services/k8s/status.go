package k8s

import (
	"encoding/json"
	"strings"

	"github.com/luuuunet/owpanel/internal/services/appstore"
)

type StatusResult struct {
	K3sInstalled    bool   `json:"k3s_installed"`
	K3sRunning      bool   `json:"k3s_running"`
	NodesReady      int    `json:"nodes_ready"`
	NodesTotal      int    `json:"nodes_total"`
	SystemPodsReady int    `json:"system_pods_ready"`
	SystemPodsTotal int    `json:"system_pods_total"`
	K8sReady        bool   `json:"k8s_ready"`
	Hint            string `json:"hint,omitempty"`
	LinuxOnly       bool   `json:"linux_only"`
}

func (s *Service) Status() (*StatusResult, error) {
	out := &StatusResult{LinuxOnly: !s.linuxHost()}
	if !s.linuxHost() {
		out.Hint = "K8s 集群管理需在 Linux 服务器上运行"
		return out, nil
	}

	out.K3sRunning = s.k3sRunning()
	out.K3sInstalled = out.K3sRunning || s.appInstalled(k3sAppKey)
	if !out.K3sInstalled {
		out.Hint = "请先在软件商店安装 k3s，或使用下方一键安装"
		return out, nil
	}
	if !out.K3sRunning {
		out.Hint = "k3s 已安装但未运行，请执行 systemctl start k3s"
		return out, nil
	}

	ready, total := s.nodeCounts()
	out.NodesReady = ready
	out.NodesTotal = total
	sysReady, sysTotal := s.systemPodCounts()
	out.SystemPodsReady = sysReady
	out.SystemPodsTotal = sysTotal

	out.K8sReady = ready > 0 && ready >= total && sysTotal > 0 && sysReady >= sysTotal
	if total == 0 {
		out.Hint = "集群节点尚未就绪，请稍后刷新"
	} else if !out.K8sReady {
		out.Hint = "部分节点或系统 Pod 未就绪，请检查 kubectl get nodes / kubectl get pods -n kube-system"
	} else {
		out.Hint = "集群运行正常，可在「工作负载」查看资源，在「加入节点」获取 Worker 命令"
	}
	return out, nil
}

func ClusterReady() bool {
	if !appstore.K3sRunning() {
		return false
	}
	s := &Service{}
	if !s.linuxHost() {
		return false
	}
	ready, total := s.nodeCounts()
	if total == 0 || ready < total {
		return false
	}
	sysReady, sysTotal := s.systemPodCounts()
	return sysTotal > 0 && sysReady >= sysTotal
}

func (s *Service) nodeCounts() (ready, total int) {
	out, err := kubectl("get", "nodes", "-o", "json")
	if err != nil {
		return 0, 0
	}
	var data struct {
		Items []struct {
			Status struct {
				Conditions []struct {
					Type   string `json:"type"`
					Status string `json:"status"`
				} `json:"conditions"`
			} `json:"status"`
		} `json:"items"`
	}
	if json.Unmarshal([]byte(out), &data) != nil {
		return 0, 0
	}
	total = len(data.Items)
	for _, n := range data.Items {
		for _, c := range n.Status.Conditions {
			if c.Type == "Ready" && c.Status == "True" {
				ready++
				break
			}
		}
	}
	return ready, total
}

func (s *Service) systemPodCounts() (ready, total int) {
	out, err := kubectl("get", "pods", "-n", "kube-system", "-o", "json")
	if err != nil {
		return 0, 0
	}
	var data struct {
		Items []struct {
			Status struct {
				Phase             string `json:"phase"`
				ContainerStatuses []struct {
					Ready bool `json:"ready"`
				} `json:"containerStatuses"`
			} `json:"status"`
		} `json:"items"`
	}
	if json.Unmarshal([]byte(out), &data) != nil {
		return 0, 0
	}
	for _, p := range data.Items {
		if strings.HasPrefix(p.Status.Phase, "Succeeded") {
			continue
		}
		total++
		allReady := len(p.Status.ContainerStatuses) > 0
		for _, cs := range p.Status.ContainerStatuses {
			if !cs.Ready {
				allReady = false
				break
			}
		}
		if allReady && p.Status.Phase == "Running" {
			ready++
		}
	}
	return ready, total
}
