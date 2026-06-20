package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/luuuunet/owpanel/internal/database"
	"github.com/luuuunet/owpanel/internal/services/panelupdate"
	"github.com/luuuunet/owpanel/internal/services/settings"
	"github.com/luuuunet/owpanel/internal/version"
)

func UpdatePanel(ctx *Context, apply bool) error {
	db, err := database.Init(ctx.DataDir)
	if err != nil {
		return err
	}
	svc := panelupdate.NewService(db, ctx.DataDir, settings.NewServiceWithDataDir(db, ctx.DataDir))
	cctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()

	info := version.Info()
	fmt.Printf("  Current version: %s\n", info["version"])
	if info["git_commit"] != "" {
		fmt.Printf("  Git commit:      %s\n", info["git_commit"])
	}

	check, err := svc.Check(cctx)
	if err != nil {
		return fmt.Errorf("check update: %w", err)
	}
	fmt.Printf("  Latest version:  %s\n", check.LatestVersion)
	if check.ReleaseURL != "" {
		fmt.Printf("  Release URL:     %s\n", check.ReleaseURL)
	}
	if !check.UpdateAvailable {
		printSuccess("Panel is up to date")
		return nil
	}
	fmt.Println(yellow("  Update available"))
	if !apply {
		fmt.Println("  Run: op update --apply")
		return nil
	}
	if !check.CanApply {
		return fmt.Errorf("cannot apply update: %s", check.ApplyReason)
	}
	fmt.Println("  Downloading and applying update (panel will restart)...")
	record, err := svc.Apply(cctx, "", "cli")
	if err != nil {
		return err
	}
	printSuccess(fmt.Sprintf("Update scheduled: %s -> %s (record #%d)", record.FromVersion, record.ToVersion, record.ID))
	return nil
}

func parseUpdateArgs(args []string) bool {
	for _, a := range args {
		switch a {
		case "--apply", "-y", "apply":
			return true
		}
	}
	return false
}

func RunUpdateCommand(args []string) error {
	ctx, err := NewContext()
	if err != nil {
		return err
	}
	return UpdatePanel(ctx, parseUpdateArgs(args))
}
