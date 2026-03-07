ifneq (,$(wildcard .env))
include .env
export
endif

.PHONY: backend frontend test smoke-third-party

backend:
	cd backend && go run ./cmd/routerd

frontend:
	npm --prefix frontend run dev

test:
	cd backend && go test ./...
	npm --prefix frontend run test

smoke-third-party:
	bash scripts/test/third_party_smoke.sh
