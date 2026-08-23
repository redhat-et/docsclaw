package fileutil

import (
	"fmt"
	"os"
	"path/filepath"
)

// WriteFileAtomically writes content to target atomically using a temp file in
// the target's directory. It preserves the given file mode, closes the temp
// file before renaming, and removes the temp file on any error.
func WriteFileAtomically(target string, content []byte, mode os.FileMode) error {
	tmpFile, err := os.CreateTemp(filepath.Dir(target), filepath.Base(target)+".*.tmp")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()

	cleanup := func() {
		_ = os.Remove(tmpPath)
	}

	if _, err := tmpFile.Write(content); err != nil {
		_ = tmpFile.Close()
		cleanup()
		return fmt.Errorf("failed to write temp file: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		cleanup()
		return fmt.Errorf("failed to close temp file: %w", err)
	}
	if err := os.Chmod(tmpPath, mode); err != nil {
		cleanup()
		return fmt.Errorf("failed to set file permissions: %w", err)
	}
	if err := os.Rename(tmpPath, target); err != nil {
		cleanup()
		return fmt.Errorf("failed to rename temp file: %w", err)
	}
	return nil
}
