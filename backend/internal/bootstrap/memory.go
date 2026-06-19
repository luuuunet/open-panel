package bootstrap

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"

	"github.com/luuuunet/owpanel/internal/services/settings"
	"github.com/shirou/gopsutil/v3/mem"
)

const memoryTuneKey = "host_memory_tune_v1"

// TotalRAMMB returns installed RAM in megabytes (0 if unknown).
func TotalRAMMB() uint64 {
	if runtime.GOOS != "linux" {
		return 0
	}
	vm, err := mem.VirtualMemory()
	if err != nil || vm == nil {
		return 0
	}
	return vm.Total / 1024 / 1024
}

// SmallMachine returns true when RAM is at or below 2 GiB.
func SmallMachine() bool {
	ram := TotalRAMMB()
	return ram > 0 && ram <= 2048
}

// TuneMemory applies swap, kernel tuning, and DB limits suited to VPS size (first run).
func TuneMemory(settingsSvc *settings.Service) {
	TuneMemoryForce(settingsSvc, false)
}

// TuneMemoryForce re-applies memory tuning when force is true.
func TuneMemoryForce(settingsSvc *settings.Service, force bool) {
	if runtime.GOOS != "linux" || settingsSvc == nil {
		return
	}
	if !force {
		all, err := settingsSvc.GetAll()
		if err == nil && all[memoryTuneKey] == "done" {
			return
		}
	}

	ramMB := TotalRAMMB()
	profile := "normal"
	if ramMB > 0 && ramMB <= 1024 {
		profile = "tiny"
	} else if ramMB > 0 && ramMB <= 2048 {
		profile = "small"
	}

	log.Printf("[bootstrap] memory tune (RAM %d MB, profile=%s)...", ramMB, profile)

	if err := ensureSwap(ramMB); err != nil {
		log.Printf("[bootstrap] swap: %v", err)
	}
	applyKernelTuning(profile)
	applyMariaDBSmallConfig(ramMB)

	data := map[string]string{
		memoryTuneKey:        "done",
		"host_memory_profile": profile,
		"host_ram_mb":         strconv.FormatUint(ramMB, 10),
	}
	if profile == "tiny" || profile == "small" {
		data["auto_ops_mem_auto_relief"] = "true"
		data["auto_ops_resource_enabled"] = "true"
		if profile == "tiny" {
			data["auto_ops_mem_threshold"] = "80"
		} else {
			data["auto_ops_mem_threshold"] = "85"
		}
	}
	if err := settingsSvc.Update(data); err != nil {
		log.Printf("[bootstrap] save memory settings: %v", err)
	}
}

func ensureSwap(ramMB uint64) error {
	swap, err := mem.SwapMemory()
	if err != nil {
		return err
	}
	if swap.Total > 0 {
		return nil
	}
	if ramMB == 0 || ramMB >= 4096 {
		return nil
	}
	sizeMB := uint64(1024)
	if ramMB <= 1024 {
		sizeMB = 1024
	} else if ramMB <= 2048 {
		sizeMB = 1536
	}
	path := "/swapfile"
	if _, err := os.Stat(path); err == nil {
		return runShell("chmod 600 " + path + " && mkswap " + path + " && swapon " + path)
	}
	script := fmt.Sprintf(
		"fallocate -l %dM %s 2>/dev/null || dd if=/dev/zero of=%s bs=1M count=%d status=progress && chmod 600 %s && mkswap %s && swapon %s",
		sizeMB, path, path, sizeMB, path, path, path,
	)
	if err := runShell(script); err != nil {
		return err
	}
	fstab := "/etc/fstab"
	content, _ := os.ReadFile(fstab)
	if !strings.Contains(string(content), path) {
		line := fmt.Sprintf("%s none swap sw 0 0\n", path)
		_ = os.WriteFile(fstab, append(content, []byte(line)...), 0644)
	}
	log.Printf("[bootstrap] created %d MB swap at %s", sizeMB, path)
	return nil
}

func applyKernelTuning(profile string) {
	swappiness := "10"
	cachePressure := "50"
	if profile == "tiny" {
		swappiness = "5"
	}
	conf := fmt.Sprintf(`# OWPanel auto-generated memory tuning
vm.swappiness=%s
vm.vfs_cache_pressure=%s
`, swappiness, cachePressure)
	path := "/etc/sysctl.d/99-owpanel-memory.conf"
	_ = os.WriteFile(path, []byte(conf), 0644)
	_ = runShell("sysctl -p " + path + " 2>/dev/null || sysctl -w vm.swappiness=" + swappiness + " vm.vfs_cache_pressure=" + cachePressure)
}

func applyMariaDBSmallConfig(ramMB uint64) {
	if ramMB == 0 || ramMB > 2048 {
		return
	}
	pool := "64M"
	maxConn := "50"
	if ramMB <= 1024 {
		pool = "32M"
		maxConn = "30"
	}
	body := fmt.Sprintf(`# OWPanel low-memory MariaDB/MySQL tuning
[mysqld]
innodb_buffer_pool_size = %s
max_connections = %s
performance_schema = OFF
`, pool, maxConn)
	for _, path := range []string{
		"/etc/mysql/mariadb.conf.d/99-owpanel-lowmem.cnf",
		"/etc/mysql/mysql.conf.d/99-owpanel-lowmem.cnf",
		"/etc/my.cnf.d/owpanel-lowmem.cnf",
	} {
		dir := path[:strings.LastIndex(path, "/")]
		_ = os.MkdirAll(dir, 0755)
		if err := os.WriteFile(path, []byte(body), 0644); err != nil {
			continue
		}
		_ = runShell("systemctl try-restart mariadb 2>/dev/null || systemctl try-restart mysql 2>/dev/null || true")
		log.Printf("[bootstrap] wrote DB low-memory config: %s", path)
		break
	}
}

func runShell(script string) error {
	cmd := exec.Command("sh", "-c", script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}
