package aisite

import (
	"strings"

	"github.com/luuuunet/owpanel/internal/services/appstore"
)

func laravelDeployScript(dataDir string, panel PanelContext) string {
	composerInstall := laravelComposerInstallBlock(dataDir, panel)
	npmBlock := laravelNPMBuildBlock(panel)

	return `#!/bin/bash
set -euo pipefail
cd "{{root}}"

` + gitCloneIntoSiteRootBlock() + `
` + composerInstall + `

if [ ! -f .env ]; then
  cp .env.example .env
fi

php artisan key:generate --force

` + npmBlock + `

php artisan config:cache
php artisan route:cache
php artisan view:cache

chmod -R 775 storage bootstrap/cache
find storage bootstrap/cache -type d -exec chmod g+s {} \; 2>/dev/null || true
`
}

func laravelComposerInstallBlock(dataDir string, panel PanelContext) string {
	if bin := strings.TrimSpace(appstore.ComposerBinary(dataDir)); bin != "" {
		return bin + ` install --no-dev --optimize-autoloader --no-interaction
`
	}
	if panel.ComposerAvail {
		return `composer install --no-dev --optimize-autoloader --no-interaction
`
	}
	return `if ! command -v composer >/dev/null 2>&1; then
  curl -sS https://getcomposer.org/installer | php
  php composer.phar install --no-dev --optimize-autoloader --no-interaction
else
  composer install --no-dev --optimize-autoloader --no-interaction
fi
`
}

func laravelNPMBuildBlock(panel PanelContext) string {
	block := npmInstallBlock() + npmBuildBlock()
	if !panel.NPMAvailable {
		return `if command -v npm >/dev/null 2>&1; then
` + block + `fi
`
	}
	return block
}

func laravelPostNotes(dataDir string, panel PanelContext, needDatabase, autoDBConfigured bool) string {
	return automatedPostNotes("laravel", panel, needDatabase && autoDBConfigured)
}
