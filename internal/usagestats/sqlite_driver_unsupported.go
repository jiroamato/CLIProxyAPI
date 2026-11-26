//go:build !((darwin && (amd64 || arm64)) || (freebsd && (386 || amd64 || arm || arm64)) || (illumos && amd64) || (linux && (386 || amd64 || arm || arm64 || loong64 || ppc64le || riscv64 || s390x)) || (netbsd && amd64) || (openbsd && (amd64 || arm64)) || (windows && (386 || amd64 || arm64)))

package usagestats

// sqliteAvailable is false on platforms modernc.org/sqlite does not support,
// such as Solaris. Usage statistics stay off there; the rest of the server runs.
const sqliteAvailable = false
