// Package pluginhost is the enforced plugin host adapter (CR-4.3 C4-25):
// it wraps the pinned WASM runtime (wazero) around the internal/plugins
// contract model so that manifest quotas, per-call authorization,
// cancellation, and fault containment are MEASURED at the product
// boundary instead of only modeled. Untrusted plugin execution stays
// DISABLED at the product level: nothing outside this package's fixtures
// instantiates a plugin, no CLI/API surface reaches here, and the D-14
// gate remains open (cross-target runtime execution and the
// publisher-signature profile are still unproven). No instruction
// fuel-metering claim is made: the CPU-millis axis has no tested runtime
// support; the wall-time Timeout axis is the enforced ceiling and is
// never labeled fuel.
package pluginhost
