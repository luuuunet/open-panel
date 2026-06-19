package k8s

import (
	"encoding/json"
	"strconv"
	"time"
)

type NodeRow struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Roles   string `json:"roles"`
	Version string `json:"version"`
	Age     string `json:"age"`
}

type PodRow struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Status    string `json:"status"`
	Ready     string `json:"ready"`
	Restarts  int    `json:"restarts"`
	Age       string `json:"age"`
	Node      string `json:"node"`
}

type DeploymentRow struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Ready     string `json:"ready"`
	UpToDate  int    `json:"up_to_date"`
	Available int    `json:"available"`
	Age       string `json:"age"`
}

type NamespaceRow struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Age    string `json:"age"`
}

func (s *Service) ListNodes() ([]NodeRow, error) {
	out, err := kubectl("get", "nodes", "-o", "json")
	if err != nil {
		return nil, err
	}
	var data struct {
		Items []struct {
			Metadata struct {
				Name              string `json:"name"`
				CreationTimestamp time.Time `json:"creationTimestamp"`
			} `json:"metadata"`
			Status struct {
				Conditions []struct {
					Type   string `json:"type"`
					Status string `json:"status"`
				} `json:"conditions"`
				NodeInfo struct {
					KubeletVersion string `json:"kubeletVersion"`
				} `json:"nodeInfo"`
			} `json:"status"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(out), &data); err != nil {
		return nil, err
	}
	rows := make([]NodeRow, 0, len(data.Items))
	for _, n := range data.Items {
		status := "NotReady"
		for _, c := range n.Status.Conditions {
			if c.Type == "Ready" {
				if c.Status == "True" {
					status = "Ready"
				} else {
					status = "NotReady"
				}
				break
			}
		}
		rolesOut, _ := kubectl("get", "node", n.Metadata.Name, "-o", "jsonpath={.metadata.labels.node-role\\.kubernetes\\.io/control-plane}")
		roles := "worker"
		if rolesOut != "" {
			roles = "control-plane,worker"
		}
		rows = append(rows, NodeRow{
			Name:    n.Metadata.Name,
			Status:  status,
			Roles:   roles,
			Version: n.Status.NodeInfo.KubeletVersion,
			Age:     formatAge(n.Metadata.CreationTimestamp),
		})
	}
	return rows, nil
}

func (s *Service) ListPods() ([]PodRow, error) {
	out, err := kubectl("get", "pods", "-A", "-o", "json")
	if err != nil {
		return nil, err
	}
	var data struct {
		Items []struct {
			Metadata struct {
				Name              string `json:"name"`
				Namespace         string `json:"namespace"`
				CreationTimestamp time.Time `json:"creationTimestamp"`
			} `json:"metadata"`
			Spec struct {
				NodeName string `json:"nodeName"`
			} `json:"spec"`
			Status struct {
				Phase             string `json:"phase"`
				ContainerStatuses []struct {
					Ready        bool `json:"ready"`
					RestartCount int  `json:"restartCount"`
				} `json:"containerStatuses"`
			} `json:"status"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(out), &data); err != nil {
		return nil, err
	}
	rows := make([]PodRow, 0, len(data.Items))
	for _, p := range data.Items {
		readyN, totalN, restarts := 0, len(p.Status.ContainerStatuses), 0
		for _, cs := range p.Status.ContainerStatuses {
			if cs.Ready {
				readyN++
			}
			restarts += cs.RestartCount
		}
		rows = append(rows, PodRow{
			Name:      p.Metadata.Name,
			Namespace: p.Metadata.Namespace,
			Status:    p.Status.Phase,
			Ready:     formatReady(readyN, totalN),
			Restarts:  restarts,
			Age:       formatAge(p.Metadata.CreationTimestamp),
			Node:      p.Spec.NodeName,
		})
	}
	return rows, nil
}

func (s *Service) ListDeployments() ([]DeploymentRow, error) {
	out, err := kubectl("get", "deployments", "-A", "-o", "json")
	if err != nil {
		return nil, err
	}
	var data struct {
		Items []struct {
			Metadata struct {
				Name              string `json:"name"`
				Namespace         string `json:"namespace"`
				CreationTimestamp time.Time `json:"creationTimestamp"`
			} `json:"metadata"`
			Status struct {
				ReadyReplicas   int `json:"readyReplicas"`
				Replicas        int `json:"replicas"`
				UpdatedReplicas int `json:"updatedReplicas"`
				AvailableReplicas int `json:"availableReplicas"`
			} `json:"status"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(out), &data); err != nil {
		return nil, err
	}
	rows := make([]DeploymentRow, 0, len(data.Items))
	for _, d := range data.Items {
		replicas := d.Status.Replicas
		if replicas == 0 {
			replicas = d.Status.ReadyReplicas
		}
		rows = append(rows, DeploymentRow{
			Name:      d.Metadata.Name,
			Namespace: d.Metadata.Namespace,
			Ready:     formatReady(d.Status.ReadyReplicas, replicas),
			UpToDate:  d.Status.UpdatedReplicas,
			Available: d.Status.AvailableReplicas,
			Age:       formatAge(d.Metadata.CreationTimestamp),
		})
	}
	return rows, nil
}

func (s *Service) ListNamespaces() ([]NamespaceRow, error) {
	out, err := kubectl("get", "namespaces", "-o", "json")
	if err != nil {
		return nil, err
	}
	var data struct {
		Items []struct {
			Metadata struct {
				Name              string `json:"name"`
				CreationTimestamp time.Time `json:"creationTimestamp"`
			} `json:"metadata"`
			Status struct {
				Phase string `json:"phase"`
			} `json:"status"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(out), &data); err != nil {
		return nil, err
	}
	rows := make([]NamespaceRow, 0, len(data.Items))
	for _, ns := range data.Items {
		rows = append(rows, NamespaceRow{
			Name:   ns.Metadata.Name,
			Status: ns.Status.Phase,
			Age:    formatAge(ns.Metadata.CreationTimestamp),
		})
	}
	return rows, nil
}

func formatReady(ready, total int) string {
	if total == 0 {
		return "0/0"
	}
	return strconv.Itoa(ready) + "/" + strconv.Itoa(total)
}

func formatAge(t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "刚刚"
	case d < time.Hour:
		return strconv.Itoa(int(d.Minutes())) + "m"
	case d < 24*time.Hour:
		return strconv.Itoa(int(d.Hours())) + "h"
	default:
		return strconv.Itoa(int(d.Hours()/24)) + "d"
	}
}
