package oci

import (
	"archive/tar"
	"bytes"
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

	// 5. Find the content layer (artifact or image mode).
	var contentDesc *ocispec.Descriptor
	for i := range manifest.Layers {
		mt := manifest.Layers[i].MediaType
		if mt == ContentMediaType || mt == ocispec.MediaTypeImageLayerGzip {
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
// It prevents path traversal via ".." components and symlinks.
func extractTarGzip(data []byte, destDir string) error {
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return fmt.Errorf("failed to create destination directory: %w", err)
	}

	// Resolve destDir to catch pre-existing symlinks at the root.
	resolvedDest, err := filepath.EvalSymlinks(destDir)
	if err != nil {
		return fmt.Errorf("failed to resolve destination: %w", err)
	}
	destPrefix := filepath.Clean(resolvedDest) + string(os.PathSeparator)

	gr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer gr.Close()

	tr := tar.NewReader(gr)

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to read tar header: %w", err)
		}

		// Reject ".." anywhere in the path.
		if strings.Contains(header.Name, "..") {
			return fmt.Errorf("invalid path in archive: %s (contains '..')", header.Name)
		}

		targetPath := filepath.Join(resolvedDest, header.Name)

		// Verify the target stays inside destDir after resolution.
		if !strings.HasPrefix(targetPath, destPrefix) {
			return fmt.Errorf("invalid path in archive: %s (outside destination)", header.Name)
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(targetPath, os.FileMode(header.Mode)); err != nil {
				return fmt.Errorf("failed to create directory %s: %w", targetPath, err)
			}

		case tar.TypeReg:
			// Resolve parent to catch symlinks in intermediate directories.
			parentDir := filepath.Dir(targetPath)
			if err := os.MkdirAll(parentDir, 0o755); err != nil {
				return fmt.Errorf("failed to create parent directory for %s: %w", targetPath, err)
			}
			resolvedParent, err := filepath.EvalSymlinks(parentDir)
			if err != nil {
				return fmt.Errorf("failed to resolve parent of %s: %w", targetPath, err)
			}
			if !strings.HasPrefix(resolvedParent, filepath.Clean(resolvedDest)) {
				return fmt.Errorf("symlink escape detected: %s resolves outside destination", targetPath)
			}

			outFile, err := os.OpenFile(targetPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(header.Mode))
			if err != nil {
				return fmt.Errorf("failed to create file %s: %w", targetPath, err)
			}
			if _, err := io.Copy(outFile, tr); err != nil {
				outFile.Close()
				return fmt.Errorf("failed to write file %s: %w", targetPath, err)
			}
			outFile.Close()

		default:
			// Skip symlinks, devices, etc.
			continue
		}
	}

	return nil
}
