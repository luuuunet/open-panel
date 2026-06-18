package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/luuuunet/owpanel/internal/services/appstore"
)

func main() {
	outDir := filepath.Join("web", "software")
	if len(os.Args) > 1 {
		outDir = os.Args[1]
	}
	created, err := appstore.EnsureSoftwareIcons(outDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "gen-software-icons: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("software icons: %d created in %s\n", created, outDir)
}
