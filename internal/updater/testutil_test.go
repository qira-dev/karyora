package updater

import (
	"os"
	"path/filepath"
)

// Tiny test-only file helpers (the production side uses os directly; these
// exist so the harness reads like the scenarios it stages).

func writeFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o755)
}

func readFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}

func mkdirAll(path string) error { return os.MkdirAll(path, 0o755) }

func filepath_dir(path string) string { return filepath.Dir(path) }

func listDir(path string) ([]string, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names, nil
}
