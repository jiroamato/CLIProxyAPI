//go:build !solaris || illumos

package usagestats

// Pure-Go SQLite driver, registered as "sqlite".
import _ "modernc.org/sqlite"

// sqliteAvailable reports whether this build includes the SQLite driver.
const sqliteAvailable = true
