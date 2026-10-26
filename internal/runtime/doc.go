// Package runtime wires the Core process modes (foreground, optional OS
// service, worker) onto one lifecycle contract with graceful shutdown and
// startup recovery ordering. It composes app, security, and platform ports;
// it must not bypass the single-authority data-directory ownership rules.
package runtime
