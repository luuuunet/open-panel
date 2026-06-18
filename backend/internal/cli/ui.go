package cli

import (
	"fmt"
	"os"
	"strings"
)

const (
	ansiReset  = "\033[0m"
	ansiBold   = "\033[1m"
	ansiDim    = "\033[2m"
	ansiCyan   = "\033[36m"
	ansiGreen  = "\033[32m"
	ansiYellow = "\033[33m"
	ansiRed    = "\033[31m"
)

func useColor() bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	fi, err := os.Stdout.Stat()
	return err == nil && (fi.Mode()&os.ModeCharDevice) != 0
}

func paint(code, s string) string {
	if !useColor() {
		return s
	}
	return code + s + ansiReset
}

func bold(s string) string  { return paint(ansiBold, s) }
func dim(s string) string    { return paint(ansiDim, s) }
func cyan(s string) string   { return paint(ansiCyan, s) }
func green(s string) string  { return paint(ansiGreen, s) }
func yellow(s string) string { return paint(ansiYellow, s) }
func red(s string) string    { return paint(ansiRed, s) }

func printBanner() {
	fmt.Println()
	fmt.Println(cyan("  ╭────────────────────────────────────────╮"))
	fmt.Println(cyan("  │") + bold("           OWPANEL  CLI               ") + cyan("│"))
	fmt.Println(cyan("  │") + dim("      Server Management Console       ") + cyan("│"))
	fmt.Println(cyan("  ╰────────────────────────────────────────╯"))
	fmt.Println()
}

func printDivider() {
	fmt.Println(dim("  ────────────────────────────────────────────"))
}

func printMenuTitle(title string) {
	fmt.Println()
	fmt.Println(bold("  " + title))
	printDivider()
}

type infoRow struct {
	label string
	value string
}

func printInfoBox(rows []infoRow) {
	width := 14
	for _, r := range rows {
		if len(r.label) > width {
			width = len(r.label)
		}
	}
	fmt.Println()
	for _, r := range rows {
		if r.value == "" {
			continue
		}
		fmt.Printf("  %s  %s\n", dim(fmt.Sprintf("%-*s", width, r.label+":")), r.value)
	}
	fmt.Println()
}

func printMenuItem(num, label, hint string) {
	line := fmt.Sprintf("  [%s]  %s", bold(num), label)
	if hint != "" {
		line += dim("  ·  "+hint)
	}
	fmt.Println(line)
}

func printSuccess(msg string) {
	fmt.Println(green("  ✓ " + msg))
}

func printError(msg string) {
	fmt.Println(red("  ✗ " + msg))
}

func printHint(msg string) {
	fmt.Println(dim("  → " + msg))
}

func clearScreen() {
	if !useColor() {
		return
	}
	fmt.Print("\033[2J\033[H")
}

func trimLower(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}
