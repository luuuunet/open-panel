package main

import (
	"log"
	"strconv"

	"github.com/luuuunet/owpanel/internal/api"
	"github.com/luuuunet/owpanel/internal/config"
	"github.com/luuuunet/owpanel/internal/database"
	"github.com/luuuunet/owpanel/internal/services/settings"
)

func main() {
	cfg := config.Load()
	db, err := database.Init(cfg.DataDir)
	if err != nil {
		log.Fatalf("database init: %v", err)
	}

	settingsSvc := settings.NewServiceWithDataDir(db, cfg.DataDir)
	if all, err := settingsSvc.GetAll(); err == nil {
		if p := all["panel_port"]; p != "" {
			if n, err := strconv.Atoi(p); err == nil && n > 0 {
				cfg.Port = n
			}
		}
	}

	server := api.NewServer(cfg, db)
	if err := server.Run(); err != nil {
		log.Fatalf("server: %v", err)
	}
}
