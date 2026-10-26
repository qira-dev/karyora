//go:build linux || darwin

package platform

import "os"

// syncDirFamily fsyncs the directory handle (POSIX).
func syncDirFamily(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}
