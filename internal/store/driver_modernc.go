//go:build !cgosqlite

package store

// Default driver: modernc.org/sqlite — pure Go, no CGO, builds anywhere
// (including Windows without gcc).
import _ "modernc.org/sqlite"

const driverName = "sqlite"

func dsn(path string) string {
	return path + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)"
}
