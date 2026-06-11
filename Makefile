ifneq (,$(wildcard .env))
include .env
export
endif

.PHONY: backend frontend server-dev test smoke-third-party package-desktop

BACKEND_DATABASE_PATH ?= $(CURDIR)/backend/data/dev.sqlite

backend:
	mkdir -p backend/data
	cd backend && CODEX_ROUTER_DATABASE_PATH="$(BACKEND_DATABASE_PATH)" go run ./cmd/routerd

server-dev:
	mkdir -p backend/data/server-dev
	cd backend && AI_GATE_MODE=server AI_GATE_SERVER_PASSWORD="$${AI_GATE_SERVER_PASSWORD:-dev-password}" CODEX_ROUTER_DATABASE_PATH="$(CURDIR)/backend/data/server-dev/aigate.sqlite" go run ./cmd/routerd --server

frontend:
	npm --prefix frontend run dev

test:
	cd backend && go test ./...
	npm --prefix frontend run test

smoke-third-party:
	bash scripts/test/third_party_smoke.sh

package-desktop:
	bash scripts/desktop/package_local_release.sh
