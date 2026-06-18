package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/luuuunet/owpanel/internal/services/settings"
)

var safePathRe = regexp.MustCompile(`^[a-zA-Z0-9_-]{4,32}$`)

func ChangePanelPort(ctx *Context) error {
	all, _ := ctx.Settings.GetAll()
	cur := ctx.Cfg.Port
	if p := all["panel_port"]; p != "" {
		if n, err := parsePort(p); err == nil {
			cur = n
		}
	}
	fmt.Printf("  Current port: %d\n", cur)
	port := readInt("  New panel port: ", cur)
	if port < 1 || port > 65535 {
		return fmt.Errorf("invalid port")
	}
	if err := ctx.Settings.Update(map[string]string{"panel_port": fmt.Sprintf("%d", port)}); err != nil {
		return err
	}
	if err := syncSystemdPort(ctx, port); err != nil {
		fmt.Printf("Warning: %v\n", err)
	}
	fmt.Printf("  Port updated to %d. Restarting panel...\n", port)
	return RestartPanel()
}

func ChangeSafePath(ctx *Context) error {
	all, _ := ctx.Settings.GetAll()
	cur := all["panel_safe_path"]
	fmt.Printf("  Current entrance: /%s\n", cur)
	fmt.Println("  Enter a new path, press Enter to auto-generate, or type 'off' to disable.")
	path := strings.Trim(readLine("  New entrance: "), "/")
	if path == "" {
		path = settings.GenerateSafePath()
		printSuccess(fmt.Sprintf("Generated entrance: /%s", path))
	} else if strings.EqualFold(path, "off") || path == "-" {
		path = ""
		printSuccess("Security entrance disabled.")
	} else if !safePathRe.MatchString(path) {
		return fmt.Errorf("invalid entrance (use 4-32 letters, numbers, _ or -)")
	}
	if err := ctx.Settings.Update(map[string]string{"panel_safe_path": path}); err != nil {
		return err
	}
	if path != "" {
		printSuccess(fmt.Sprintf("Entrance set to: /%s", path))
	}
	printHint("Restart the panel, then run: op info")
	return nil
}

func TogglePanelSSL(ctx *Context) error {
	all, _ := ctx.Settings.GetAll()
	on := all["panel_ssl"] == "true"
	fmt.Printf("  Panel SSL is currently: %v\n", on)
	next := "true"
	if on {
		next = "false"
	}
	if err := ctx.Settings.Update(map[string]string{"panel_ssl": next}); err != nil {
		return err
	}
	if next == "true" {
		printSuccess("SSL enabled in settings. Configure certificate or reverse proxy separately.")
	} else {
		printSuccess("SSL disabled in settings.")
	}
	return nil
}

func syncSystemdPort(ctx *Context, port int) error {
	if runtime.GOOS != "linux" || !hasSystemctl() {
		return nil
	}
	unitPath := filepath.Join("/etc/systemd/system", serviceName()+".service")
	data, err := os.ReadFile(unitPath)
	if err != nil {
		return err
	}
	content := string(data)
	if !strings.Contains(content, "OWPANEL_PORT=") {
		return fmt.Errorf("systemd unit has no OWPANEL_PORT")
	}
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "Environment=OWPANEL_PORT=") {
			lines[i] = fmt.Sprintf("Environment=OWPANEL_PORT=%d", port)
		}
	}
	if err := os.WriteFile(unitPath, []byte(strings.Join(lines, "\n")), 0644); err != nil {
		return err
	}
	cmd := exec.Command("systemctl", "daemon-reload")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func ForceChangeMySQLRoot(_ *Context) error {
	if _, err := exec.LookPath("mysql"); err != nil {
		return fmt.Errorf("mysql client not found — install MySQL/MariaDB first")
	}
	pass := readPassword("  New MySQL root password: ")
	if pass == "" {
		return fmt.Errorf("password cannot be empty")
	}
	cmd := exec.Command("mysqladmin", "-u", "root", "password", pass)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("mysqladmin failed (try logging in as root first): %w", err)
	}
	printSuccess("MySQL root password updated.")
	return nil
}
