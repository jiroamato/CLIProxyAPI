//go:build solaris && !illumos

package usagestats

// sqliteAvailable is false because modernc.org/sqlite does not build for
// Solaris. Usage statistics stay disabled there; the rest of the server runs.
// GOOS=illumos also matches the solaris tag but is supported by the driver.
const sqliteAvailable = false
