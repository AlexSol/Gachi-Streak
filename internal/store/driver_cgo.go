//go:build cgosqlite

package store

// Alternative driver: github.com/mattn/go-sqlite3 (needs CGO and gcc).
// Build with: go build -tags cgosqlite ./cmd/server
import _ "github.com/mattn/go-sqlite3"

const driverName = "sqlite3"

func dsn(path string) string {
	return path + "?_busy_timeout=5000&_journal_mode=WAL&_foreign_keys=on"
}
