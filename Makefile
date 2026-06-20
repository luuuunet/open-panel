VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
BUILD_DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
GIT_COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
LDFLAGS = -X github.com/luuuunet/owpanel/internal/version.Version=$(VERSION) \
          -X github.com/luuuunet/owpanel/internal/version.BuildDate=$(BUILD_DATE) \
          -X github.com/luuuunet/owpanel/internal/version.GitCommit=$(GIT_COMMIT)

.PHONY: dev build backend frontend docker install

dev: backend-dev frontend-dev

backend-dev:
	cd backend && go run ./cmd/server

frontend-dev:
	cd frontend && npm run dev

build: backend-build frontend-build

backend-build:
	cd backend && go build -ldflags "$(LDFLAGS)" -o bin/owpanel ./cmd/server
	cd backend && go build -ldflags "$(LDFLAGS)" -o bin/op ./cmd/op

frontend-build:
	cd frontend && npm run build

docker:
	docker compose build

install:
	bash scripts/install.sh

setup:
	bash scripts/auto-setup.sh build

deploy:
	bash scripts/auto-setup.sh deploy

release:
	@test -n "$(VERSION)" || (echo "Usage: make release VERSION=v0.1.16 [RELEASE_ARGS='--draft']" && exit 1)
	bash scripts/publish-github-release.sh $(VERSION) $(RELEASE_ARGS)

release-ci:
	@test -n "$(VERSION)" || (echo "Usage: make release-ci VERSION=v0.1.16" && exit 1)
	bash scripts/publish-github-release.sh $(VERSION) --ci
