// Package platform contains OS lifecycle and path adapters: user data,
// config, cache, log, and temp locations; foreground/service integration for
// Linux, Windows, and macOS. OS-specific APIs are imported HERE only; domain
// and application code must not reach this package. Browser or native client
// launch decisions belong to the separate UI track (D-26/D-39).
package platform
