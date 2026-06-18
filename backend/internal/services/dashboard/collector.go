package dashboard

import (
	"runtime"
	"strings"
	"time"

	"github.com/luuuunet/owpanel/internal/models"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/load"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/shirou/gopsutil/v3/net"
)

const cpuSampleMs = 100
const snapshotPersistEvery = 2
const snapshotPruneEvery = 5
const defaultCollectInterval = 15 * time.Second
const sampleHostCacheTTL = 5 * time.Minute
const sampleDiskCacheTTL = 60 * time.Second

type rawSample struct {
	at        time.Time
	cpu       float64
	cores     int
	memoryPct float64
	vm        *mem.VirtualMemoryStat
	swap      *mem.SwapMemoryStat
	load      LoadStats
	disks     []DiskStats
	netSent   uint64
	netRecv   uint64
	diskRead  uint64
	diskWrite uint64
	host      *host.InfoStat
}

func (s *Service) sampleRaw() rawSample {
	now := time.Now()
	cpuPercent, _ := cpu.Percent(cpuSampleMs*time.Millisecond, false)
	cores, hostInfo := s.cachedHostMeta(now)

	vmStat, _ := mem.VirtualMemory()
	swapStat, _ := mem.SwapMemory()
	disks := s.cachedDiskStats(now)
	diskIO, _ := disk.IOCounters()

	var cpuUsage float64
	if len(cpuPercent) > 0 {
		cpuUsage = cpuPercent[0]
	}

	sent, recv := aggregateNetBytes()
	var totalRead, totalWrite uint64
	for name, io := range diskIO {
		if skipDiskDevice(name) {
			continue
		}
		totalRead += io.ReadBytes
		totalWrite += io.WriteBytes
	}

	var vm mem.VirtualMemoryStat
	var swap mem.SwapMemoryStat
	if vmStat != nil {
		vm = *vmStat
	}
	if swapStat != nil {
		swap = *swapStat
	}
	var hi host.InfoStat
	if hostInfo != nil {
		hi = *hostInfo
	}

	return rawSample{
		at: now, cpu: cpuUsage, cores: cores, memoryPct: vm.UsedPercent,
		vm: &vm, swap: &swap, load: readLoad(cores, cpuUsage),
		disks: disks, netSent: sent, netRecv: recv,
		diskRead: totalRead, diskWrite: totalWrite, host: &hi,
	}
}

func (s *Service) cachedHostMeta(now time.Time) (cores int, hostInfo *host.InfoStat) {
	s.sampleMetaMu.Lock()
	defer s.sampleMetaMu.Unlock()
	if !s.sampleMetaAt.IsZero() && now.Sub(s.sampleMetaAt) < sampleHostCacheTTL {
		return s.sampleCores, &s.sampleHost
	}
	cores, _ = cpu.Counts(true)
	if hi, err := host.Info(); err == nil && hi != nil {
		s.sampleHost = *hi
	}
	s.sampleCores = cores
	s.sampleMetaAt = now
	return cores, &s.sampleHost
}

func (s *Service) cachedDiskStats(now time.Time) []DiskStats {
	s.sampleMetaMu.Lock()
	if !s.sampleDisksAt.IsZero() && now.Sub(s.sampleDisksAt) < sampleDiskCacheTTL && len(s.sampleDisks) > 0 {
		out := make([]DiskStats, len(s.sampleDisks))
		copy(out, s.sampleDisks)
		s.sampleMetaMu.Unlock()
		return out
	}
	s.sampleMetaMu.Unlock()

	partitions, _ := disk.Partitions(false)
	disks := make([]DiskStats, 0, len(partitions))
	for _, p := range partitions {
		if shouldSkipDiskMount(p.Mountpoint, p.Fstype) {
			continue
		}
		usage, err := disk.Usage(p.Mountpoint)
		if err != nil {
			continue
		}
		disks = append(disks, DiskStats{
			Mount: p.Mountpoint, Total: usage.Total, Used: usage.Used,
		 Free: usage.Free, UsedPercent: usage.UsedPercent,
		})
	}

	s.sampleMetaMu.Lock()
	s.sampleDisks = disks
	s.sampleDisksAt = now
	s.sampleMetaMu.Unlock()
	return disks
}

func (s *Service) ratesFromSample(cur rawSample) (upload, download, readRate, writeRate float64) {
	s.mu.Lock()
	prev := s.prev
	s.mu.Unlock()
	if prev == nil {
		return 0, 0, 0, 0
	}
	dt := cur.at.Sub(prev.at).Seconds()
	if dt <= 0 {
		return 0, 0, 0, 0
	}
	return rateDelta(cur.netSent, prev.netSent, dt),
		rateDelta(cur.netRecv, prev.netRecv, dt),
		rateDelta(cur.diskRead, prev.diskRead, dt),
		rateDelta(cur.diskWrite, prev.diskWrite, dt)
}

func (s *Service) collectAndStore() {
	cur := s.sampleRaw()
	up, down, readRate, writeRate := s.ratesFromSample(cur)

	point := MetricPoint{
		Time: cur.at.Unix(), CPU: round2(cur.cpu), Memory: round2(cur.memoryPct),
		Load1: round2(cur.load.Load1), NetUp: round2(up), NetDown: round2(down),
		DiskRead: round2(readRate), DiskWrite: round2(writeRate),
	}

	s.mu.Lock()
	s.prev = &rateSnapshot{
		at: cur.at, netSent: cur.netSent, netRecv: cur.netRecv,
		diskRead: cur.diskRead, diskWrite: cur.diskWrite,
	}
	s.history = append(s.history, point)
	if len(s.history) > maxHistoryPoints {
		s.history = s.history[len(s.history)-maxHistoryPoints:]
	}
	s.latest = cur
	s.latestRates = ioRates{up: up, down: down, read: readRate, write: writeRate}
	s.hasSample = true
	s.mu.Unlock()

	if s.db != nil {
		s.samplesSincePersist++
		if s.samplesSincePersist >= snapshotPersistEvery {
			s.samplesSincePersist = 0
			row := models.MetricSnapshot{
				CreatedAt: cur.at, CPU: point.CPU, Memory: point.Memory, Load1: point.Load1,
				NetUp: point.NetUp, NetDown: point.NetDown, DiskRead: point.DiskRead, DiskWrite: point.DiskWrite,
			}
			_ = s.db.Create(&row).Error
			s.persistCount++
			if s.persistCount%snapshotPruneEvery == 0 {
				s.pruneSnapshots()
			}
		}
	}
}

func (s *Service) StartCollector() {
	go func() {
		s.pruneSnapshots()
		s.collectAndStore()
		for {
			interval := defaultCollectInterval
			if s.perf != nil {
				interval = s.perf.CollectInterval()
			}
			timer := time.NewTimer(interval)
			select {
			case <-timer.C:
			}
			timer.Stop()
			s.collectAndStore()
		}
	}()
}

func (s *Service) pruneSnapshots() {
	if s.db == nil {
		return
	}
	cutoff := time.Now().Add(-24 * time.Hour)
	for i := 0; i < 20; i++ {
		res := s.db.Where("created_at < ?", cutoff).Limit(5000).Delete(&models.MetricSnapshot{})
		if res.Error != nil || res.RowsAffected == 0 {
			break
		}
	}
}

func readLoad(cores int, cpuPct float64) LoadStats {
	if avg, err := load.Avg(); err == nil && (avg.Load1 > 0 || runtime.GOOS == "linux") {
		return LoadStats{Load1: avg.Load1, Load5: avg.Load5, Load15: avg.Load15}
	}
	approx := (cpuPct / 100) * float64(cores)
	return LoadStats{Load1: approx, Load5: approx, Load15: approx}
}

func aggregateNetBytes() (sent, recv uint64) {
	counters, err := net.IOCounters(true)
	if err != nil || len(counters) == 0 {
		all, err2 := net.IOCounters(false)
		if err2 != nil || len(all) == 0 {
			return 0, 0
		}
		return all[0].BytesSent, all[0].BytesRecv
	}
	for _, c := range counters {
		if skipNetInterface(c.Name) {
			continue
		}
		sent += c.BytesSent
		recv += c.BytesRecv
	}
	return sent, recv
}

func skipNetInterface(name string) bool {
	n := strings.ToLower(name)
	switch {
	case n == "lo", n == "lo0":
		return true
	case strings.Contains(n, "loopback"):
		return true
	case strings.HasPrefix(n, "veth"), strings.HasPrefix(n, "docker"), strings.HasPrefix(n, "br-"):
		return true
	case strings.HasPrefix(n, "isatap"), strings.HasPrefix(n, "teredo"):
		return true
	case strings.Contains(n, "virtual"), strings.Contains(n, "vmware"), strings.Contains(n, "hyper-v"):
		return true
	}
	return false
}

func skipDiskDevice(name string) bool {
	n := strings.ToLower(name)
	return strings.HasPrefix(n, "loop") || strings.HasPrefix(n, "ram")
}

func shouldSkipDiskMount(mount, fstype string) bool {
	if runtime.GOOS == "windows" {
		return false
	}
	switch mount {
	case "/boot", "/boot/efi", "/snap", "/run", "/dev", "/proc", "/sys":
		return true
	}
	if strings.HasPrefix(mount, "/snap/") {
		return true
	}
	fs := strings.ToLower(fstype)
	return fs == "tmpfs" || fs == "devtmpfs" || fs == "squashfs" || fs == "overlay"
}

func rawToStats(cur rawSample, rates ioRates) *Stats {
	var vmTotal, vmUsed, vmFree uint64
	var vmPct float64
	if cur.vm != nil {
		vmTotal, vmUsed, vmFree = cur.vm.Total, cur.vm.Used, cur.vm.Free
		vmPct = cur.vm.UsedPercent
	}
	var swapTotal, swapUsed, swapFree uint64
	var swapPct float64
	if cur.swap != nil {
		swapTotal, swapUsed, swapFree = cur.swap.Total, cur.swap.Used, cur.swap.Free
		swapPct = cur.swap.UsedPercent
	}
	var hostname, osName, platform, platformVer string
	var uptime uint64
	if cur.host != nil {
		hostname, osName, platform = cur.host.Hostname, cur.host.OS, cur.host.Platform
		platformVer, uptime = cur.host.PlatformVersion, cur.host.Uptime
	}
	return &Stats{
		CPU:     CPUStats{UsagePercent: cur.cpu, Cores: cur.cores},
		Memory:  MemoryStats{Total: vmTotal, Used: vmUsed, Free: vmFree, UsedPercent: vmPct},
		Swap:    SwapStats{Total: swapTotal, Used: swapUsed, Free: swapFree, UsedPercent: swapPct},
		Load:    cur.load,
		Disk:    cur.disks,
		DiskIO:  DiskIOStats{ReadBytes: cur.diskRead, WriteBytes: cur.diskWrite, ReadRate: rates.read, WriteRate: rates.write},
		Network: NetworkStats{BytesSent: cur.netSent, BytesRecv: cur.netRecv, UploadRate: rates.up, DownloadRate: rates.down},
		System:  SystemInfo{Hostname: hostname, OS: osName, Platform: platform, PlatformVersion: platformVer, Uptime: uptime},
	}
}

type ioRates struct {
	up, down, read, write float64
}
