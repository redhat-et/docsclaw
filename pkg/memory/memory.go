package memory

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Store is a long-term memory backend. Implementations may be file-backed,
// database-backed, or remote.
type Store interface {
	// Load returns the current memory content, or an empty string if no
	// memory has been recorded yet.
	Load(ctx context.Context) (string, error)
	// Remember appends a new entry to the memory store.
	Remember(ctx context.Context, entry string) error
}

// FileStore appends memory entries to a Markdown file on disk.
type FileStore struct {
	path string
}

// NewFileStore creates a file-backed memory store. The file is created on
// the first Remember call if it does not exist.
func NewFileStore(path string) *FileStore {
	return &FileStore{path: path}
}

// Load reads the memory file. A missing file is treated as empty memory.
func (s *FileStore) Load(_ context.Context) (string, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("read memory file: %w", err)
	}
	return string(data), nil
}

// Remember appends the entry to the memory file with a timestamp header.
// The file is created with owner-only permissions (0600) and existing files
// are chmod'd to 0600 on every call to guard against umask or prior leaks.
func (s *FileStore) Remember(_ context.Context, entry string) (err error) {
	entry = strings.TrimSpace(entry)
	if entry == "" {
		return nil
	}

	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create memory directory: %w", err)
	}

	f, err := os.OpenFile(s.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("open memory file: %w", err)
	}
	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("close memory file: %w", cerr)
		}
	}()

	// Ensure owner-only permissions regardless of umask or pre-existing mode.
	if err := os.Chmod(s.path, 0600); err != nil {
		return fmt.Errorf("chmod memory file: %w", err)
	}

	// Determine whether the existing content ends with a newline by reading
	// only the final byte instead of the whole file.
	prefix := ""
	if _, err := f.Seek(-1, io.SeekEnd); err == nil {
		buf := make([]byte, 1)
		if _, err := f.Read(buf); err == nil && buf[0] != '\n' {
			prefix = "\n"
		}
	}

	section := fmt.Sprintf("%s## %s\n\n%s\n\n",
		prefix,
		time.Now().UTC().Format(time.RFC3339),
		entry,
	)

	if _, err = f.WriteString(section); err != nil {
		return fmt.Errorf("write memory file: %w", err)
	}
	return err
}
