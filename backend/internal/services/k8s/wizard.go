package k8s

import (
	"fmt"
	"strings"
	"time"
)

const sampleDeployment = "owpanel-nginx-demo"

type WizardResult struct {
	Message string   `json:"message"`
	Steps   []string `json:"steps"`
	Install *InstallResult `json:"install,omitempty"`
}

func (s *Service) RunWizard(deploySample bool) (*WizardResult, error) {
	if !s.linuxHost() {
		return nil, fmt.Errorf("K8s 向导仅支持 Linux 服务器")
	}
	if ram := s.totalRAMMB(); ram > 0 && ram <= 2048 {
		return nil, fmt.Errorf("服务器内存仅 %d MB，不建议运行 K3s（建议 ≥4GB）。请升级配置或在面板「首页」使用一键优化释放内存", ram)
	}
	res := &WizardResult{Steps: []string{}}

	if !s.k3sRunning() {
		inst, err := s.Install()
		if err != nil {
			return nil, err
		}
		res.Install = inst
		res.Steps = append(res.Steps, "已安装 K3s")
		time.Sleep(3 * time.Second)
	} else {
		res.Steps = append(res.Steps, "K3s 已在运行")
	}

	st, err := s.Status()
	if err != nil {
		return nil, err
	}
	if st.K8sReady {
		res.Steps = append(res.Steps, "集群节点与系统 Pod 已就绪")
	} else {
		res.Steps = append(res.Steps, fmt.Sprintf("集群验证：节点 %d/%d，系统 Pod %d/%d",
			st.NodesReady, st.NodesTotal, st.SystemPodsReady, st.SystemPodsTotal))
	}

	if deploySample {
		if err := s.deploySampleApp(); err != nil {
			return nil, fmt.Errorf("示例应用部署失败: %w", err)
		}
		res.Steps = append(res.Steps, "已部署示例 nginx（owpanel-nginx-demo）")
	}

	res.Message = "K8s 向导完成：可在「工作负载」查看资源，在「加入节点」复制 Worker 加入命令"
	return res, nil
}

func (s *Service) deploySampleApp() error {
	if s.sampleAppDeployed() {
		return nil
	}
	if _, err := kubectl("create", "deployment", sampleDeployment, "--image=nginx:alpine", "--replicas=1"); err != nil {
		if !strings.Contains(err.Error(), "AlreadyExists") {
			return err
		}
	}
	_, err := kubectl("expose", "deployment", sampleDeployment, "--port=80", "--type=ClusterIP")
	if err != nil && !strings.Contains(err.Error(), "AlreadyExists") {
		return err
	}
	return nil
}

func (s *Service) DeploySample() error {
	if !s.k3sRunning() {
		return fmt.Errorf("k3s 未运行")
	}
	return s.deploySampleApp()
}
