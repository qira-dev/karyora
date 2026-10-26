//go:build windows

package platform

// syncDirFamily: the stdlib cannot open a directory handle on Windows, so
// the directory-entry flush is not performed here. The Windows publish
// durability boundary (file write-through only) is the recorded per-target
// evidence item for the C2-12 adapter — never a claimed guarantee.
func syncDirFamily(dir string) error { return nil }
