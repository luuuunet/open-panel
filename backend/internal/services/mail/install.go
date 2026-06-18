package mail

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/luuuunet/owpanel/internal/platform"
)

func (s *Service) InstallStack() error {
	if runtime.GOOS == "windows" {
		return fmt.Errorf("邮件服务器需在 Linux 主机上安装")
	}
	pkgs := []string{
		"postfix", "dovecot-core", "dovecot-imapd", "dovecot-pop3d", "mailutils", "ssl-cert",
	}
	if err := installPackages(pkgs); err != nil {
		return err
	}
	if err := s.ensureVMailUser(); err != nil {
		return err
	}
	if err := s.applyPostfixMain(); err != nil {
		return err
	}
	if err := s.syncMaps(); err != nil {
		return err
	}
	_ = exec.Command("systemctl", "enable", "postfix", "dovecot").Run()
	if out, err := exec.Command("systemctl", "restart", "postfix").CombinedOutput(); err != nil {
		return fmt.Errorf("restart postfix: %s", strings.TrimSpace(string(out)))
	}
	if out, err := exec.Command("systemctl", "restart", "dovecot").CombinedOutput(); err != nil {
		return fmt.Errorf("restart dovecot: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

func (s *Service) UninstallStack() error {
	if runtime.GOOS == "windows" {
		return fmt.Errorf("Windows 不支持卸载邮件栈")
	}
	_ = exec.Command("systemctl", "stop", "postfix", "dovecot").Run()
	_ = exec.Command("systemctl", "disable", "postfix", "dovecot").Run()
	return removePackages([]string{
		"postfix", "dovecot-core", "dovecot-imapd", "dovecot-pop3d", "mailutils",
	})
}

// EnsureConfigured applies panel mail maps after Postfix/Dovecot install from software store.
func (s *Service) EnsureConfigured() error {
	if runtime.GOOS == "windows" {
		return nil
	}
	if !commandExists("postfix") && !commandExists("dovecot") {
		return nil
	}
	if err := s.ensureVMailUser(); err != nil {
		return err
	}
	if commandExists("postconf") {
		_ = s.applyPostfixMain()
	}
	return s.syncMaps()
}

func installPackages(pkgs []string) error {
	mgr := platform.PackageManager()
	switch mgr {
	case "apt":
		if out, err := exec.Command("apt-get", "update", "-qq").CombinedOutput(); err != nil {
			return fmt.Errorf("apt update: %s", strings.TrimSpace(string(out)))
		}
		args := append([]string{"install", "-y", "-qq"}, pkgs...)
		cmd := exec.Command("apt-get", args...)
		cmd.Env = append(os.Environ(), "DEBIAN_FRONTEND=noninteractive")
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("apt install: %s", strings.TrimSpace(string(out)))
		}
		return nil
	case "dnf", "yum":
		args := append([]string{"install", "-y"}, pkgs...)
		if out, err := exec.Command(mgr, args...).CombinedOutput(); err != nil {
			return fmt.Errorf("%s install: %s", mgr, strings.TrimSpace(string(out)))
		}
		return nil
	default:
		return fmt.Errorf("不支持的包管理器（需要 apt/dnf/yum）")
	}
}

func removePackages(pkgs []string) error {
	mgr := platform.PackageManager()
	switch mgr {
	case "apt":
		args := append([]string{"remove", "-y", "--purge"}, pkgs...)
		if out, err := exec.Command("apt-get", args...).CombinedOutput(); err != nil {
			return fmt.Errorf("apt remove: %s", strings.TrimSpace(string(out)))
		}
		return nil
	case "dnf", "yum":
		args := append([]string{"remove", "-y"}, pkgs...)
		if out, err := exec.Command(mgr, args...).CombinedOutput(); err != nil {
			return fmt.Errorf("%s remove: %s", mgr, strings.TrimSpace(string(out)))
		}
		return nil
	default:
		return fmt.Errorf("不支持的包管理器")
	}
}

func commandExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}
