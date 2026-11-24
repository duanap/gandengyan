.PHONY: test simulate e2e build build-linux

test:
	cd server && go test ./... && go vet ./...
	cd web && npm ci && npm run typecheck && npm test

simulate:
	mkdir -p dist
	cd server && go run ./cmd/simulate -per-group 1250 -seed 13001 > ../dist/simulation.jsonl

e2e: build
	cd web && npm run e2e

build:
	node scripts/build.mjs

build-linux:
	node scripts/build.mjs --linux
