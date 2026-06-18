package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/luuuunet/owpanel/internal/auth"
)

func ChangePanelPassword(ctx *Context) error {
	user := readLine("  Username (Enter = admin): ")
	if user == "" {
		user = "admin"
	}
	pass := readPassword("  New password: ")
	if len(pass) < 8 {
		return fmt.Errorf("password must be at least 8 characters")
	}
	if err := ctx.Auth.ChangePasswordByUsername(user, pass); err != nil {
		return err
	}
	printSuccess(fmt.Sprintf("Password updated for %s", user))
	return nil
}

func ChangePanelUsername(ctx *Context) error {
	oldName := readLine("  Current username: ")
	if oldName == "" {
		oldName = "admin"
	}
	newName := readLine("  New username: ")
	if newName == "" {
		return fmt.Errorf("username cannot be empty")
	}
	if err := ctx.Auth.ChangeUsername(oldName, newName); err != nil {
		return err
	}
	printSuccess(fmt.Sprintf("Username changed: %s → %s", oldName, newName))
	return nil
}

func ClearLoginLimit(_ *Context) error {
	auth.ClearLoginLimits()
	printSuccess("Login lockout records cleared.")
	return nil
}

func ShowPanelErrorLog(_ *Context) error {
	printMenuTitle("Service Log (last 40 lines)")
	if err := runJournalTail(serviceName(), 40); err != nil {
		return fmt.Errorf("journalctl not available: %w", err)
	}
	return nil
}

func RunPanelConfiguration(ctx *Context) error {
	for {
		printMenuTitle("Panel Configuration")
		printMenuItem("1", "Change panel port", "")
		printMenuItem("2", "Security entrance path", "")
		printMenuItem("3", "Toggle SSL setting", "")
		printMenuItem("0", "Back", "")
		fmt.Println()
		choice := strings.TrimSpace(readLine("  Choice: "))
		if choice == "" {
			continue
		}
		var runErr error
		switch choice {
		case "0":
			return nil
		case "1":
			runErr = ChangePanelPort(ctx)
		case "2":
			runErr = ChangeSafePath(ctx)
		case "3":
			runErr = TogglePanelSSL(ctx)
		default:
			printError("Unknown option: " + choice)
		}
		if runErr != nil {
			printError(runErr.Error())
		}
		if choice != "0" {
			pause()
		}
	}
}

func printMainMenu() {
	printMenuTitle("Main Menu")
	printMenuItem("1", "Panel information", "URLs, port, data directory")
	printMenuItem("2", "Panel configuration", "port · entrance · SSL")
	printMenuItem("3", "Change admin password", "")
	printMenuItem("4", "Change admin username", "")
	printMenuItem("5", "Restart panel", "")
	printMenuItem("6", "Stop panel", "")
	printMenuItem("7", "Start panel", "")
	printMenuItem("8", "View service log", "")
	printMenuItem("9", "Clear panel cache", "")
	printMenuItem("10", "Clear login lockout", "")
	printMenuItem("11", "Repair panel", "")
	printMenuItem("12", "Reset MySQL root password", "")
	printMenuItem("13", "Uninstall panel", "Remove service and program files")
	printMenuItem("0", "Exit", "")
	if os.Getuid() != 0 {
		fmt.Println()
		printHint("Service control (restart/stop/logs) may require: sudo op")
	}
	fmt.Println()
}

func RunMenu() error {
	ctx, err := NewContext()
	if err != nil {
		return err
	}

	clearScreen()
	printBanner()
	_ = ShowDefaultInfo(ctx)

	for {
		printMainMenu()
		choice := strings.TrimSpace(readLine("  Choice: "))
		if choice == "" {
			continue
		}
		var runErr error
		switch choice {
		case "0":
			fmt.Println()
			printHint("Goodbye.")
			return nil
		case "1":
			runErr = ShowDefaultInfo(ctx)
		case "2":
			runErr = RunPanelConfiguration(ctx)
		case "3":
			runErr = ChangePanelPassword(ctx)
		case "4":
			runErr = ChangePanelUsername(ctx)
		case "5":
			runErr = RestartPanel()
		case "6":
			runErr = StopPanel()
		case "7":
			runErr = StartPanel()
		case "8":
			runErr = ShowPanelErrorLog(ctx)
		case "9":
			runErr = ClearPanelCache(ctx)
		case "10":
			runErr = ClearLoginLimit(ctx)
		case "11":
			runErr = RepairPanel(ctx)
		case "12":
			runErr = ForceChangeMySQLRoot(ctx)
		case "13":
			runErr = UninstallPanel(ctx)
			if runErr == nil {
				return nil
			}
		default:
			printError("Unknown option: " + choice)
		}
		if runErr != nil {
			printError(runErr.Error())
		}
		if choice != "0" && choice != "2" {
			pause()
		}
		fmt.Println()
	}
}

func printHelp() {
	printBanner()
	fmt.Println(bold("  Usage"))
	printDivider()
	fmt.Println("  op              Interactive menu (shows panel info first)")
	fmt.Println("  op info         Show panel URLs and settings")
	fmt.Println("  op config       Panel configuration submenu")
	fmt.Println("  op restart      Restart the panel service")
	fmt.Println("  op stop         Stop the panel service")
	fmt.Println("  op start        Start the panel service")
	fmt.Println("  op repair       Run panel repair checks")
	fmt.Println("  op uninstall    Remove panel service and files (sudo)")
	fmt.Println("  op help         Show this help")
	fmt.Println()
}

func RunCommand(arg string) error {
	ctx, err := NewContext()
	if err != nil {
		return err
	}
	switch trimLower(arg) {
	case "help", "-h", "--help":
		printHelp()
		return nil
	case "info", "1":
		printBanner()
		return ShowDefaultInfo(ctx)
	case "config", "2":
		printBanner()
		return RunPanelConfiguration(ctx)
	case "password", "3":
		return ChangePanelPassword(ctx)
	case "username", "4":
		return ChangePanelUsername(ctx)
	case "restart", "5":
		return RestartPanel()
	case "stop", "6":
		return StopPanel()
	case "start", "7":
		return StartPanel()
	case "logs", "8":
		return ShowPanelErrorLog(ctx)
	case "repair", "11":
		return RepairPanel(ctx)
	case "uninstall", "13":
		return UninstallPanel(ctx)
	default:
		return fmt.Errorf("unknown command %q (try: op help)", arg)
	}
}
