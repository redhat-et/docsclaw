package oci

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content/memory"
	"oras.land/oras-go/v2/registry"
)

// PullOptions configures the pull operation.
type PullOptions struct {
	TLSVerify *bool // nil = default (true), false = plain HTTP
	Registry  oras.Target
}

// Pull fetches a skill artifact from a registry and extracts it to destDir.
func Pull(ctx context.Context, ref, destDir string, opts PullOptions) error {
	// 1. Resolve target registry
	target, err := resolveTarget(ref, opts.Registry, opts.TLSVerify)
	if err != nil {
		return fmt.Errorf("failed to resolve target: %w", err)
	}

	// 2. Parse reference
	parsedRef, err := registry.ParseReference(ref)
	if err != nil {
		return fmt.Errorf("failed to parse reference: %w", err)
	}

	// 3. Copy from target to local memory store
	localStore := memory.New()
	copyOpts := oras.DefaultCopyOptions
	copyOpts.Concurrency = 1
	desc, err := oras.Copy(ctx, target, parsedRef.Reference, localStore, parsedRef.Reference, copyOpts)
	if err != nil {
		return fmt.Errorf("failed to copy from registry: %w", err)
	}

	// 4. Fetch and parse the manifest
	manifestData, err := fetchBlob(ctx, localStore, desc)
	if err != nil {
		return fmt.Errorf("failed to fetch manifest: %w", err)
	}

	var manifest ocispec.Manifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		return fmt.Errorf("failed to unmarshal manifest: %w", err)
	}

	// 5. Find the content layer (ContentMediaType)
	var contentDesc *ocispec.Descriptor
	for i := range manifest.Layers {
		if manifest.Layers[i].MediaType == ContentMediaType {
			contentDesc = &manifest.Layers[i]
			break
		}
	}

	if contentDesc == nil {
		return fmt.Errorf("content layer not found in manifest")
	}

	// 6. Fetch the content layer
	contentData, err := fetchBlob(ctx, localStore, *contentDesc)
	if err != nil {
		return fmt.Errorf("failed to fetch content layer: %w", err)
	}

	// 7. Extract the tar+gzip to destDir
	if err := extractTarGzip(contentData, destDir); err != nil {
		return fmt.Errorf("failed to extract content: %w", err)
	}

	return nil
}

// fetchBlob reads a blob from storage given its descriptor.
func fetchBlob(ctx context.Context, storage oras.ReadOnlyTarget, desc ocispec.Descriptor) ([]byte, error) {
	rc, err := storage.Fetch(ctx, desc)
	if err != nil {
		return nil, err
	}
	defer rc.Close()

	data, err := io.ReadAll(rc)
	if err != nil {
		return nil, err
	}

	return data, nil
}

// extractTarGzip extracts a tar+gzip archive to the destination directory.
// It prevents path traversal attacks by rejecting paths containing "..".
func extractTarGzip(data []byte, destDir string) error {
	// Create destination directory if it doesn't exist
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("failed to create destination directory: %w", err)
	}

	// Create gzip reader
	gr, err := gzip.NewReader(strings.NewReader(string(data)))
	if err != nil {
		return fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer gr.Close()

	// Create tar reader
	tr := tar.NewReader(gr)

	// Extract each file
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to read tar header: %w", err)
		}

		// Prevent path traversal - reject any path with ".."
		if strings.Contains(header.Name, "..") {
			return fmt.Errorf("invalid path in archive: %s (contains '..')", header.Name)
		}

		// Build target path
		targetPath := filepath.Join(destDir, header.Name)

		// Ensure the target path is within destDir (additional safety check)
		if !strings.HasPrefix(targetPath, filepath.Clean(destDir)+string(os.PathSeparator)) {
			return fmt.Errorf("invalid path in archive: %s (outside destination)", header.Name)
		}

		switch header.Typeflag {
		case tar.TypeDir:
			// Create directory
			if err := os.MkdirAll(targetPath, os.FileMode(header.Mode)); err != nil {
				return fmt.Errorf("failed to create directory %s: %w", targetPath, err)
			}

		case tar.TypeReg:
			// Create parent directory
			if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
				return fmt.Errorf("failed to create parent directory for %s: %w", targetPath, err)
			}

			// Create file
			outFile, err := os.OpenFile(targetPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(header.Mode))
			if err != nil {
				return fmt.Errorf("failed to create file %s: %w", targetPath, err)
			}

			// Copy file content
			if _, err := io.Copy(outFile, tr); err != nil {
				outFile.Close()
				return fmt.Errorf("failed to write file %s: %w", targetPath, err)
			}

			outFile.Close()

		default:
			// Skip other types (symlinks, etc.)
			continue
		}
	}

	return nil
}
