package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// UninstallPanel stops the service and removes panel files from the server.
func UninstallPanel(ctx *Context) error {
	if runtime.GOOS != "linux" {
		return fmt.Errorf("uninstall is supported on Linux only")
	}
	if os.Geteuid() != 0 {
		return fmt.Errorf("run as root: sudo op uninstall")
	}

	install := ctx.Install
	if install == "" {
		install = "/opt/owpanel"
	}
	dataDir := ctx.DataDir
	if dataDir == "" {
		dataDir = filepath.Join(install, "data")
	}

	printMenuTitle("Uninstall OWPanel")
	fmt.Printf("  Install dir: %s\n", install)
	fmt.Printf("  Data dir:    %s\n", dataDir)
	fmt.Println()
	printError("This removes the panel service and program files from this server.")
	fmt.Println()

	confirm := strings.TrimSpace(readLine("  Type UNINSTALL to continue: "))
	if confirm != "UNINSTALL" {
		printHint("Uninstall cancelled.")
		return nil
	}

	keepAns := strings.ToLower(strings.TrimSpace(readLine("  Keep website/database data? (Y/n): ")))
	keepData := keepAns != "n" && keepAns != "no"

	if err := stopAndRemoveService(); err != nil {
		return err
	}
	removeOpSymlink(install)

	if keepData {
		if err := removePanelProgramFiles(install, dataDir); err != nil {
			return err
		}
		printSuccess("OWPanel uninstalled. Program files removed.")
		fmt.Printf("  Data preserved at: %s\n", dataDir)
		printHint("To reinstall later, run the install script — existing data may be reused.")
	} else {
		if err := os.RemoveAll(install); err != nil {
			return fmt.Errorf("remove install dir: %w", err)
		}
		printSuccess("OWPanel fully removed (including data).")
	}

	fmt.Println()
	printHint("Firewall rules for the panel port were not changed automatically.")
	return nil
}

func stopAndRemoveService() error {
	name := serviceName()
	if hasSystemctl() {
		_ = exec.Command("systemctl", "stop", name).Run()
		_ = exec.Command("systemctl", "disable", name).Run()
	}
	unitPath := filepath.Join("/etc/systemd/system", name+".service")
	if err := os.Remove(unitPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove systemd unit: %w", err)
	}
	if hasSystemctl() {
		cmd := exec.Command("systemctl", "daemon-reload")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("systemctl daemon-reload: %w", err)
		}
	}
	printSuccess(fmt.Sprintf("Service %q stopped and disabled", name))
	return nil
}

func removeOpSymlink(install string) {
	const globalOp = "/usr/local/bin/op"
	target, err := os.Readlink(globalOp)
	if err != nil {
		return
	}
	expected := filepath.Join(install, "op")
	if target == expected || filepath.Clean(target) == filepath.Clean(expected) {
		if err := os.Remove(globalOp); err != nil && !os.IsNotExist(err) {
			fmt.Println(yellow(fmt.Sprintf("  ! Could not remove %s: %v", globalOp, err)))
			return
		}
		printSuccess("Removed /usr/local/bin/op")
	}
}

func removePanelProgramFiles(install, dataDir string) error {
	dataDir = filepath.Clean(dataDir)
	install = filepath.Clean(install)

	for _, name := range []string{"owpanel", "op", "web", "logs", "bt"} {
		p := filepath.Join(install, name)
		if filepath.Clean(p) == dataDir {
			continue
		}
		if err := os.RemoveAll(p); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove %s: %w", p, err)
		}
	}

	entries, err := os.ReadDir(install)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, e := range entries {
		p := filepath.Join(install, e.Name())
		if filepath.Clean(p) == dataDir {
			continue
		}
		if err := os.RemoveAll(p); err != nil {
			return fmt.Errorf("remove %s: %w", p, err)
		}
	}
	return nil
}
