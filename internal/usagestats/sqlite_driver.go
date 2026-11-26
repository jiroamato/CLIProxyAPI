//go:build (darwin && (amd64 || arm64)) || (freebsd && (386 || amd64 || arm || arm64)) || (illumos && amd64) || (linux && (386 || amd64 || arm || arm64 || loong64 || ppc64le || riscv64 || s390x)) || (netbsd && amd64) || (openbsd && (amd64 || arm64)) || (windows && (386 || amd64 || arm64))

package usagestats

// Pure-Go SQLite driver, registered as "sqlite". The build constraint lists the
// platforms modernc.org/sqlite supports; update it when that module adds one.
import _ "modernc.org/sqlite"

// sqliteAvailable reports whether this build includes the SQLite driver.
const sqliteAvailable = true
