package platform

import (
	"os"
)

// SyncFile flushes one file to durable storage (POSIX fsync; Windows
// write-through FlushFileBuffers via os.File.Sync).
func SyncFile(f *os.File) error {
	return f.Sync()
}

// SyncDir flushes a directory entry so a completed rename survives a power
// loss (C2-12 §2 per-target durability: POSIX fsyncs the directory; the
// Windows family records its own publish durability evidence — the stdlib
// cannot open a directory handle there, so the Windows implementation
// returns without a directory flush and that boundary is part of the
// recorded per-target evidence, never a claimed guarantee).
func SyncDir(dir string) error {
	return syncDirFamily(dir)
}
