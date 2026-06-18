package main

import (
	"fmt"
	"os"

	"github.com/luuuunet/owpanel/internal/config"
	"github.com/luuuunet/owpanel/internal/database"
	"github.com/luuuunet/owpanel/internal/services/aichat"
	"github.com/luuuunet/owpanel/internal/services/settings"
)

func main() {
	cfg := config.Load()
	db, err := database.Init(cfg.DataDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open db: %v\n", err)
		os.Exit(1)
	}

	settingsSvc := settings.NewService(db)
	aiSvc := aichat.NewService(settingsSvc)

	all, _ := settingsSvc.GetAll()
	fmt.Println("=== Panel AI Settings ===")
	for _, k := range []string{"ai_enabled", "ai_provider", "ai_base_url", "ai_model"} {
		fmt.Printf("  %s = %q\n", k, all[k])
	}
	if all["ai_api_key"] != "" {
		fmt.Printf("  ai_api_key = set (%d chars)\n", len(all["ai_api_key"]))
	} else {
		fmt.Println("  ai_api_key = (empty)")
	}

	st := aiSvc.AssistantStatus()
	fmt.Printf("\n=== Assistant Status ===\n  enabled=%v configured=%v provider=%q model=%q\n  message=%q\n",
		st.Enabled, st.Configured, st.Provider, st.Model, st.Message)

	if key := os.Getenv("CURSOR_API_KEY"); key != "" {
		fmt.Println("\n=== Test with CURSOR_API_KEY env ===")
		testKey(aiSvc, key, all["ai_base_url"], all["ai_model"])
		return
	}
	if all["ai_api_key"] == "" {
		fmt.Println("\nNo API key in panel. Set CURSOR_API_KEY env to test manually.")
		os.Exit(1)
	}
	fmt.Println("\n=== Cursor API Tests (panel key) ===")
	testKey(aiSvc, all["ai_api_key"], all["ai_base_url"], all["ai_model"])
}

func testKey(aiSvc *aichat.Service, key, baseURL, model string) {
	fmt.Println("\n1) List models (GET /v1/models)...")
	models, err := aiSvc.ListModels("cursor", key, baseURL)
	if err != nil {
		fmt.Printf("   FAIL: %v\n", err)
	} else {
		fmt.Printf("   OK: %d models", len(models))
		if len(models) > 0 {
			fmt.Printf(" (first: %s)", models[0].ID)
		}
		fmt.Println()
	}

	fmt.Println("\n2) Chat probe (POST /v1/agents, no-repo)...")
	reply, err := aiSvc.TestCursorChat(key, baseURL, model)
	if err != nil {
		fmt.Printf("   FAIL: %v\n", err)
		os.Exit(1)
	}
	preview := reply
	if len(preview) > 200 {
		preview = preview[:200] + "..."
	}
	fmt.Printf("   OK: %q\n", preview)
}
