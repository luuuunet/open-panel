package enterprise

import (
	"github.com/luuuunet/owpanel/internal/models"
	"github.com/luuuunet/owpanel/internal/services/dashboard"
)

type MonitoringNodeMetric struct {
	ID         uint    `json:"id"`
	Name       string  `json:"name"`
	Host       string  `json:"host"`
	Role       string  `json:"role"`
	Status     string  `json:"status"`
	IsLocal    bool    `json:"is_local"`
	CPUPercent float64 `json:"cpu_percent"`
	MemPercent float64 `json:"mem_percent"`
	DiskPercent float64 `json:"disk_percent"`
	Load1      float64 `json:"load1"`
}

type UptimeSummary struct {
	Total   int `json:"total"`
	Up      int `json:"up"`
	Down    int `json:"down"`
	Unknown int `json:"unknown"`
	Enabled int `json:"enabled"`
}

type AdvancedMonitoring struct {
	LocalHostname string                 `json:"local_hostname"`
	Nodes         []MonitoringNodeMetric   `json:"nodes"`
	Uptime        UptimeSummary            `json:"uptime"`
	ClusterOnline int                      `json:"cluster_online"`
	ClusterTotal  int                      `json:"cluster_total"`
}

func (s *Service) GetAdvancedMonitoring() (AdvancedMonitoring, error) {
	out := AdvancedMonitoring{Nodes: []MonitoringNodeMetric{}}
	if s.dashboard != nil {
		if st, err := s.dashboard.GetStats(); err == nil && st != nil {
			out.LocalHostname = st.System.Hostname
			diskPct := maxDiskPct(st.Disk)
			out.Nodes = append(out.Nodes, MonitoringNodeMetric{
				Name: "本机", Host: "local", Role: "master", Status: "online", IsLocal: true,
				CPUPercent: st.CPU.UsagePercent, MemPercent: st.Memory.UsedPercent,
				DiskPercent: diskPct, Load1: st.Load.Load1,
			})
		}
	}
	var nodes []models.ClusterNode
	s.db.Order("is_local desc, id asc").Find(&nodes)
	for _, n := range nodes {
		if n.IsLocal {
			if len(out.Nodes) > 0 && out.Nodes[0].IsLocal {
				out.Nodes[0].ID = n.ID
				out.Nodes[0].Name = n.Name
				out.Nodes[0].Host = n.Host
				out.Nodes[0].CPUPercent = n.CPUPercent
				out.Nodes[0].MemPercent = n.MemPercent
				out.Nodes[0].DiskPercent = n.DiskPercent
				out.Nodes[0].Load1 = n.Load1
			}
			continue
		}
		out.Nodes = append(out.Nodes, MonitoringNodeMetric{
			ID: n.ID, Name: n.Name, Host: n.Host, Role: n.Role, Status: n.Status,
			CPUPercent: n.CPUPercent, MemPercent: n.MemPercent,
			DiskPercent: n.DiskPercent, Load1: n.Load1,
		})
		if n.Status == "online" {
			out.ClusterOnline++
		}
		out.ClusterTotal++
	}
	if s.uptime != nil {
		list, _ := s.uptime.List()
		sum := UptimeSummary{Total: len(list)}
		for _, m := range list {
			if m.Enabled {
				sum.Enabled++
			}
			switch m.LastStatus {
			case "up":
				sum.Up++
			case "down":
				sum.Down++
			default:
				sum.Unknown++
			}
		}
		out.Uptime = sum
	}
	return out, nil
}

func maxDiskPct(disks []dashboard.DiskStats) float64 {
	max := 0.0
	for _, d := range disks {
		if d.UsedPercent > max {
			max = d.UsedPercent
		}
	}
	return max
}
