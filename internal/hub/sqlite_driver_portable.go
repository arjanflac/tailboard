//go:build !darwin

package hub

import _ "modernc.org/sqlite"

const sqliteDriverName = "sqlite"
