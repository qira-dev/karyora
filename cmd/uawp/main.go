// Command uawp is the composition root of the Karyora Core.
//
// Scaffold entry point: it links no runtime behavior yet and exists so the
// module graph, dependency direction, and build pipeline have a root to
// verify against. Lifecycle and diagnostics commands are specified by the
// Phase 2 lifecycle-contract task and arrive with its implementation.
package main

import (
	"fmt"
	"os"
)

// version is the scaffold build identifier. Real, reproducible build
// metadata is stamped by the build pipeline defined in the Phase 2
// build-evidence task (C2-03); this constant is not a release version.
const version = "0.0.0-scaffold"

func main() {
	fmt.Fprintf(os.Stdout, "karyora core %s\n", version)
}
