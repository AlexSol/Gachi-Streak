.PHONY: deps test server server-win server-linux server-linux-arm64 client client-win all clean \
	web web-deps web-dev run dev unlock-all lock-all

deps:
	go mod tidy

test: deps
	go test ./...

# --- Frontend (Vite + Preact, embedded into the server via go:embed) ---

# Reinstall only when the lockfile changes.
frontend/node_modules/.package-lock.json: frontend/package-lock.json
	cd frontend && npm ci

web-deps: frontend/node_modules/.package-lock.json

# Build into internal/webui/dist (picked up by every server build below).
web: web-deps
	cd frontend && npm run build

# Vite dev server on :5173, proxies the API to a server on :8080 (override with GACHI_API).
web-dev: web-deps
	cd frontend && npm run dev

# --- Run ---

run: deps
	go run ./cmd/server serve

# --- Achievements for testing (on prod: fly ssh console -C "/gachi-server achievements unlock") ---
# make unlock-all NAME=alex   (no NAME = every user; DB=path, default gachi.db)
DB ?= gachi.db
# Only NAME from the command line: WSL exports NAME=<hostname>.
user_flag = $(if $(filter command line,$(origin NAME)),-name $(NAME))

unlock-all:
	go run ./cmd/server achievements unlock -db $(DB) $(user_flag)

lock-all:
	go run ./cmd/server achievements lock -db $(DB) $(user_flag)

# Server + Vite dev server together; open http://localhost:5173/app/
dev:
	$(MAKE) -j2 run web-dev

server: deps web
	go build -o bin/gachi-server ./cmd/server

server-win: deps web
	GOOS=windows GOARCH=amd64 go build -o bin/gachi-server.exe ./cmd/server

# Static Linux binaries (pure-Go SQLite, no CGO) — run on any distro.
server-linux: deps web
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o bin/gachi-server-linux-amd64 ./cmd/server

server-linux-arm64: deps web
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags "-s -w" -o bin/gachi-server-linux-arm64 ./cmd/server

# Client version shown in the tray (ℹ️ Про програму); the git hash is embedded by go build.
VERSION ?= 0.1.0

client-win: deps
	GOOS=windows GOARCH=amd64 go build -ldflags "-H windowsgui -s -w -X main.version=$(VERSION)" -o bin/gachi-client.exe ./cmd/client

all: server server-win server-linux server-linux-arm64 client-win

clean:
	rm -rf bin
