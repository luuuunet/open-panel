package main

import (
	"fmt"
	"log"
	"os"

	"github.com/luuuunet/owpanel/internal/config"
	"github.com/luuuunet/owpanel/internal/database"
)

func main() {
	cfg := config.Load()
	db, err := database.Init(cfg.DataDir)
	if err != nil {
		log.Fatal(err)
	}
	user := "admin"
	pass := "admin"
	if len(os.Args) > 1 {
		pass = os.Args[1]
	}
	if err := database.ResetAdminPassword(db, user, pass); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Password for user %q has been reset.\n", user)
}
