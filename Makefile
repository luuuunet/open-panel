.PHONY: dev build backend frontend docker install

dev: backend-dev frontend-dev

backend-dev:
	cd backend && go run ./cmd/server

frontend-dev:
	cd frontend && npm run dev

build: backend-build frontend-build

backend-build:
	cd backend && go build -o bin/owpanel ./cmd/server

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
