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
	bash scripts/dev/server_dev.sh

frontend:
	npm --prefix frontend run dev

test:
	cd backend && go test ./...
	npm --prefix frontend run test

smoke-third-party:
	bash scripts/test/third_party_smoke.sh

package-desktop:
	bash scripts/desktop/package_local_release.sh
