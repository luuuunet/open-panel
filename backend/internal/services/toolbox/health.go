package toolbox

import (
	"fmt"
	"strings"
	"time"

	"github.com/luuuunet/owpanel/internal/models"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/load"
	"github.com/shirou/gopsutil/v3/mem"
)

type HealthFactor struct {
	Key    string  `json:"key"`
	Label  string  `json:"label"`
	Score  int     `json:"score"`
	Max    int     `json:"max"`
	Detail string  `json:"detail"`
	Status string  `json:"status"`
	Value  float64 `json:"value,omitempty"`
}

type HealthReport struct {
	Score   int            `json:"score"`
	Grade   string         `json:"grade"`
	Summary string         `json:"summary"`
	Factors []HealthFactor `json:"factors"`
}

func (s *Service) HealthReport(lang string) (*HealthReport, error) {
	if lang == "" {
		lang = "zh"
	}
	factors := make([]HealthFactor, 0, 6)
	total := 0
	maxTotal := 0

	memScore, memFactor := scoreMemory(lang)
	factors = append(factors, memFactor)
	total += memScore
	maxTotal += 25

	diskScore, diskFactor := scoreDisk(lang)
	factors = append(factors, diskFactor)
	total += diskScore
	maxTotal += 25

	loadScore, loadFactor := scoreLoad(lang)
	factors = append(factors, loadFactor)
	total += loadScore
	maxTotal += 20

	upScore, upFactor := scoreUptime(lang)
	factors = append(factors, upFactor)
	total += upScore
	maxTotal += 10

	sslScore, sslFactor := s.scoreSSL(lang)
	factors = append(factors, sslFactor)
	total += sslScore
	maxTotal += 10

	svcScore, svcFactor := s.scoreServices(lang)
	factors = append(factors, svcFactor)
	total += svcScore
	maxTotal += 10

	score := 0
	if maxTotal > 0 {
		score = total * 100 / maxTotal
	}
	grade, summary := gradeHealth(score, lang)
	return &HealthReport{Score: score, Grade: grade, Summary: summary, Factors: factors}, nil
}

func scoreMemory(lang string) (int, HealthFactor) {
	vm, err := mem.VirtualMemory()
	f := HealthFactor{Key: "memory", Max: 25}
	if lang == "en" {
		f.Label = "Memory"
	} else {
		f.Label = "内存"
	}
	if err != nil || vm == nil {
		f.Score, f.Status, f.Detail = 15, "warn", "—"
		return 15, f
	}
	f.Value = vm.UsedPercent
	switch {
	case vm.UsedPercent >= 95:
		f.Score, f.Status = 5, "danger"
	case vm.UsedPercent >= 85:
		f.Score, f.Status = 12, "warn"
	case vm.UsedPercent >= 70:
		f.Score, f.Status = 18, "ok"
	default:
		f.Score, f.Status = 25, "ok"
	}
	f.Detail = fmt.Sprintf("%.1f%%", vm.UsedPercent)
	return f.Score, f
}

func scoreDisk(lang string) (int, HealthFactor) {
	f := HealthFactor{Key: "disk", Max: 25}
	if lang == "en" {
		f.Label = "Disk"
	} else {
		f.Label = "磁盘"
	}
	partitions, err := disk.Partitions(false)
	if err != nil {
		f.Score, f.Status, f.Detail = 15, "warn", "—"
		return 15, f
	}
	maxPct := 0.0
	mount := "/"
	for _, p := range partitions {
		if strings.HasPrefix(p.Mountpoint, "/snap") {
			continue
		}
		u, err := disk.Usage(p.Mountpoint)
		if err != nil || u.Total == 0 {
			continue
		}
		if u.UsedPercent > maxPct {
			maxPct = u.UsedPercent
			mount = p.Mountpoint
		}
	}
	f.Value = maxPct
	switch {
	case maxPct >= 95:
		f.Score, f.Status = 5, "danger"
	case maxPct >= 85:
		f.Score, f.Status = 12, "warn"
	case maxPct >= 75:
		f.Score, f.Status = 18, "ok"
	default:
		f.Score, f.Status = 25, "ok"
	}
	f.Detail = fmt.Sprintf("%s %.1f%%", mount, maxPct)
	return f.Score, f
}

func scoreLoad(lang string) (int, HealthFactor) {
	f := HealthFactor{Key: "load", Max: 20}
	if lang == "en" {
		f.Label = "Load"
	} else {
		f.Label = "负载"
	}
	cores, _ := cpu.Counts(true)
	if cores <= 0 {
		cores = 1
	}
	ld, err := load.Avg()
	if err != nil || ld == nil {
		f.Score, f.Status, f.Detail = 14, "ok", "—"
		return 14, f
	}
	ratio := ld.Load1 / float64(cores)
	f.Value = ratio
	switch {
	case ratio >= 2:
		f.Score, f.Status = 4, "danger"
	case ratio >= 1.2:
		f.Score, f.Status = 10, "warn"
	case ratio >= 0.8:
		f.Score, f.Status = 16, "ok"
	default:
		f.Score, f.Status = 20, "ok"
	}
	f.Detail = fmt.Sprintf("%.2f / %d核", ld.Load1, cores)
	if lang == "en" {
		f.Detail = fmt.Sprintf("%.2f / %d cores", ld.Load1, cores)
	}
	return f.Score, f
}

func scoreUptime(lang string) (int, HealthFactor) {
	f := HealthFactor{Key: "uptime", Max: 10}
	if lang == "en" {
		f.Label = "Uptime"
	} else {
		f.Label = "运行时间"
	}
	info, err := host.Info()
	if err != nil || info == nil {
		f.Score, f.Status, f.Detail = 8, "ok", "—"
		return 8, f
	}
	f.Detail = formatUptime(info.Uptime)
	if info.Uptime < 3600 {
		f.Score, f.Status = 6, "warn"
	} else {
		f.Score, f.Status = 10, "ok"
	}
	return f.Score, f
}

func (s *Service) scoreSSL(lang string) (int, HealthFactor) {
	f := HealthFactor{Key: "ssl", Max: 10}
	if lang == "en" {
		f.Label = "SSL certs"
	} else {
		f.Label = "SSL 证书"
	}
	if s.db == nil {
		f.Score, f.Status, f.Detail = 10, "ok", "—"
		return 10, f
	}
	var total int64
	s.db.Model(&models.SSLCertificate{}).Count(&total)
	if total == 0 {
		f.Score, f.Status = 10, "ok"
		if lang == "en" {
			f.Detail = "No certs"
		} else {
			f.Detail = "暂无证书"
		}
		return 10, f
	}
	soon := time.Now().Add(14 * 24 * time.Hour)
	var expiring int64
	s.db.Model(&models.SSLCertificate{}).Where("expires_at IS NOT NULL AND expires_at <= ?", soon).Count(&expiring)
	f.Value = float64(expiring)
	switch {
	case expiring >= 3:
		f.Score, f.Status = 3, "danger"
	case expiring >= 1:
		f.Score, f.Status = 6, "warn"
	default:
		f.Score, f.Status = 10, "ok"
	}
	if lang == "en" {
		f.Detail = fmt.Sprintf("%d total, %d expiring soon", total, expiring)
	} else {
		f.Detail = fmt.Sprintf("共 %d 张，%d 张即将过期", total, expiring)
	}
	return f.Score, f
}

func (s *Service) scoreServices(lang string) (int, HealthFactor) {
	f := HealthFactor{Key: "services", Max: 10}
	if lang == "en" {
		f.Label = "Services"
	} else {
		f.Label = "软件服务"
	}
	if s.db == nil {
		f.Score, f.Status, f.Detail = 10, "ok", "—"
		return 10, f
	}
	var installed, running int64
	s.db.Model(&models.App{}).Where("installed = ?", true).Count(&installed)
	s.db.Model(&models.App{}).Where("installed = ? AND status = ?", true, "running").Count(&running)
	if installed == 0 {
		f.Score, f.Status = 10, "ok"
		if lang == "en" {
			f.Detail = "No apps"
		} else {
			f.Detail = "无已装软件"
		}
		return 10, f
	}
	ratio := float64(running) / float64(installed)
	f.Value = ratio * 100
	switch {
	case ratio >= 0.9:
		f.Score, f.Status = 10, "ok"
	case ratio >= 0.7:
		f.Score, f.Status = 7, "warn"
	default:
		f.Score, f.Status = 4, "danger"
	}
	if lang == "en" {
		f.Detail = fmt.Sprintf("%d/%d running", running, installed)
	} else {
		f.Detail = fmt.Sprintf("%d/%d 运行中", running, installed)
	}
	return f.Score, f
}

func gradeHealth(score int, lang string) (grade, summary string) {
	switch {
	case score >= 90:
		grade = "A"
		if lang == "en" {
			summary = "System is in excellent shape"
		} else {
			summary = "系统状态优秀"
		}
	case score >= 75:
		grade = "B"
		if lang == "en" {
			summary = "System is healthy with minor issues"
		} else {
			summary = "系统整体健康，有少量待优化项"
		}
	case score >= 60:
		grade = "C"
		if lang == "en" {
			summary = "Some resources need attention"
		} else {
			summary = "部分资源需要关注"
		}
	default:
		grade = "D"
		if lang == "en" {
			summary = "Critical issues detected — review factors below"
		} else {
			summary = "存在明显风险，请查看下方分项"
		}
	}
	return grade, summary
}
