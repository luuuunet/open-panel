package autops

import (
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/luuuunet/owpanel/internal/models"
)

func (s *Service) loadMemReliefEnabled(cfg Config) bool {
	all, err := s.settings.GetAll()
	if err != nil {
		return true
	}
	if all["auto_ops_mem_auto_relief"] == "false" {
		return false
	}
	return cfg.MemAutoRelief
}

func (s *Service) checkMemoryPressure(cfg Config, now time.Time) {
	if runtime.GOOS != "linux" || s.dashboard == nil {
		return
	}
	if !s.loadMemReliefEnabled(cfg) {
		return
	}

	mon := s.dashboard.GetMonitor(0)
	if mon.Current == nil {
		return
	}
	cur := mon.Current

	memPct := cur.Memory.UsedPercent
	swapUsedMB := cur.Swap.Used / 1024 / 1024
	swapPct := cur.Swap.UsedPercent

	needsRelief := false
	reason := ""
	if memPct >= 88 {
		needsRelief = true
		reason = "内存使用率 " + formatPct(memPct)
	} else if cur.Swap.Total > 0 && swapUsedMB >= 64 && swapPct >= 20 {
		needsRelief = true
		reason = "Swap 占用 " + strconv.FormatUint(swapUsedMB, 10) + " MB (" + formatPct(swapPct) + ")"
	} else if cur.Swap.Total > 0 && swapUsedMB >= 256 {
		needsRelief = true
		reason = "Swap 占用过高 " + strconv.FormatUint(swapUsedMB, 10) + " MB"
	}
	if !needsRelief {
		return
	}

	reliefCooldown := 5 * time.Minute
	if s.inGlobalEventCooldown("memory_relief", now, reliefCooldown) {
		return
	}

	beforeMem := memPct
	beforeSwap := swapUsedMB

	if _, err := s.dashboard.FreeMemory(); err != nil {
		s.logGlobalEvent("memory_relief", "system", "系统", "自动释放内存失败: "+err.Error(), "failed")
		return
	}

	swapMsg := ""
	if swapUsedMB >= 128 {
		if out, err := runMemShell("sync; echo 3 > /proc/sys/vm/drop_caches 2>/dev/null; swapoff -a 2>/dev/null && swapon -a 2>/dev/null; echo OK"); err != nil {
			swapMsg = "；Swap 刷新失败"
		} else if strings.Contains(out, "OK") {
			swapMsg = "；已刷新 Swap"
		}
	}

	after := s.dashboard.GetMonitor(0)
	afterMem := beforeMem
	afterSwap := beforeSwap
	if after.Current != nil {
		afterMem = after.Current.Memory.UsedPercent
		afterSwap = after.Current.Swap.Used / 1024 / 1024
	}

	msg := reason + " → 已自动释放缓存" + swapMsg +
		"（内存 " + formatPct(beforeMem) + " → " + formatPct(afterMem) +
		"，Swap " + strconv.FormatUint(beforeSwap, 10) + "MB → " + strconv.FormatUint(afterSwap, 10) + "MB）"
	s.logGlobalEvent("memory_relief", "system", "系统", msg, formatPct(afterMem))
	s.maybeNotify(cfg, models.App{Key: "system", Name: "系统"}, "memory_relief", msg, formatPct(afterMem))
}

func runMemShell(script string) (string, error) {
	cmd := exec.Command("sh", "-c", script)
	out, err := cmd.CombinedOutput()
	return string(out), err
}
