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
	"sort"
	"strings"
	"time"

	"github.com/opencontainers/go-digest"
	"github.com/opencontainers/image-spec/specs-go"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/redhat-et/docsclaw/pkg/skills/card"
	"oras.land/oras-go/v2/content"
)

// PackOptions controls how the skill is packed into an OCI artifact.
type PackOptions struct {
	AsImage bool // If true, use image config media type instead of artifact type
}

// skillConfig represents the config blob stored in the OCI artifact.
type skillConfig struct {
	Name         string   `json:"name"`
	Version      string   `json:"version"`
	Description  string   `json:"description"`
	License      string   `json:"license,omitempty"`
	AllowedTools string   `json:"allowedTools,omitempty"`
	Required     []string `json:"required,omitempty"`
}

// Pack creates an OCI artifact from a skill directory and pushes it to the target storage.
// Returns the manifest descriptor.
func Pack(ctx context.Context, skillDir string, target content.Storage, opts PackOptions) (ocispec.Descriptor, error) {
	// 1. Parse and validate skill.yaml
	skillYAMLPath := filepath.Join(skillDir, "skill.yaml")
	sc, err := card.Parse(skillYAMLPath)
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("failed to parse skill card: %w", err)
	}

	// 2. Build layers and config.
	// In artifact mode: two layers (SkillCard YAML + content tarball).
	// In image mode: single tar+gzip layer with all content (like FROM scratch).
	var layers []ocispec.Descriptor
	var configData []byte
	var configMediaType string

	if opts.AsImage {
		// Single layer: tar+gzip of entire skill directory.
		tar, err := tarDirectory(skillDir, sc.Metadata.Name)
		if err != nil {
			return ocispec.Descriptor{}, fmt.Errorf("failed to create tarball: %w", err)
		}

		contentDesc, err := pushBlob(ctx, target, ocispec.MediaTypeImageLayerGzip, tar.gzipped)
		if err != nil {
			return ocispec.Descriptor{}, fmt.Errorf("failed to push content layer: %w", err)
		}
		layers = []ocispec.Descriptor{contentDesc}

		// Valid OCI image config with rootfs (required by quay.io).
		configMediaType = ImageConfigMediaType
		imgCfg := ocispec.Image{
			Platform: ocispec.Platform{
				Architecture: "unknown",
				OS:           "unknown",
			},
			RootFS: ocispec.RootFS{
				Type:    "layers",
				DiffIDs: []digest.Digest{tar.uncompressedDigest},
			},
		}
		configData, err = json.Marshal(imgCfg)
		if err != nil {
			return ocispec.Descriptor{}, fmt.Errorf("failed to marshal image config: %w", err)
		}
	} else {
		// Layer 0: skill.yaml as SkillCard metadata.
		skillYAMLData, err := os.ReadFile(skillYAMLPath)
		if err != nil {
			return ocispec.Descriptor{}, fmt.Errorf("failed to read skill.yaml: %w", err)
		}
		cardDesc, err := pushBlob(ctx, target, CardMediaType, skillYAMLData)
		if err != nil {
			return ocispec.Descriptor{}, fmt.Errorf("failed to push card layer: %w", err)
		}

		// Layer 1: tar+gzip of entire skill directory.
		tar, err := tarDirectory(skillDir, sc.Metadata.Name)
		if err != nil {
			return ocispec.Descriptor{}, fmt.Errorf("failed to create tarball: %w", err)
		}
		contentDesc, err := pushBlob(ctx, target, ContentMediaType, tar.gzipped)
		if err != nil {
			return ocispec.Descriptor{}, fmt.Errorf("failed to push content layer: %w", err)
		}
		layers = []ocispec.Descriptor{cardDesc, contentDesc}

		// Skill-specific config with metadata for community tools.
		configMediaType = ConfigMediaType
		cfg := buildConfig(sc)
		configData, err = json.Marshal(cfg)
		if err != nil {
			return ocispec.Descriptor{}, fmt.Errorf("failed to marshal config: %w", err)
		}
	}

	configDesc, err := pushBlob(ctx, target, configMediaType, configData)
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("failed to push config blob: %w", err)
	}

	// 4. Build manifest with annotations
	annotations := buildAnnotations(sc)

	manifest := ocispec.Manifest{
		Versioned: specs.Versioned{SchemaVersion: 2},
		MediaType: ocispec.MediaTypeImageManifest,
		Config:    configDesc,
		Layers:    layers,
		Annotations: annotations,
	}

	// Set artifactType only when NOT in AsImage mode
	if !opts.AsImage {
		manifest.ArtifactType = ArtifactType
	}

	// 6. Push manifest
	manifestData, err := json.Marshal(manifest)
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("failed to marshal manifest: %w", err)
	}

	manifestDesc, err := pushBlob(ctx, target, ocispec.MediaTypeImageManifest, manifestData)
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("failed to push manifest: %w", err)
	}

	return manifestDesc, nil
}

// buildConfig creates a skillConfig from a SkillCard.
func buildConfig(sc card.SkillCard) skillConfig {
	return skillConfig{
		Name:         sc.Metadata.Name,
		Version:      sc.Metadata.Version,
		Description:  sc.Metadata.Description,
		License:      sc.Metadata.License,
		AllowedTools: sc.Spec.AllowedTools,
		Required:     sc.Spec.Tools.Required,
	}
}

// pushBlob creates a descriptor and pushes data to the target storage.
func pushBlob(ctx context.Context, target content.Storage, mediaType string, data []byte) (ocispec.Descriptor, error) {
	desc := content.NewDescriptorFromBytes(mediaType, data)
	if err := target.Push(ctx, desc, bytes.NewReader(data)); err != nil {
		return ocispec.Descriptor{}, err
	}
	return desc, nil
}

// tarResult holds the output of tarDirectory.
type tarResult struct {
	gzipped            []byte
	uncompressedDigest digest.Digest
}

// tarDirectory creates a deterministic tar+gzip of the skill directory.
// All entries are rooted at <skillName>/ and have a fixed mtime for reproducibility.
// Returns the gzipped data and the digest of the uncompressed tar (for rootfs.diff_ids).
func tarDirectory(dir, skillName string) (tarResult, error) {
	// Write tar to an intermediate buffer to compute the uncompressed digest.
	var tarBuf bytes.Buffer
	tw := tar.NewWriter(&tarBuf)

	// Fixed mtime for reproducible digests (2026-01-01 00:00:00 UTC)
	fixedTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	// Directories to skip when packing (generated artifacts, not skill content).
	skipDirs := map[string]bool{
		"oci-layout": true,
	}

	// Collect all file paths and sort them
	var paths []string
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() && skipDirs[info.Name()] {
			return filepath.SkipDir
		}
		paths = append(paths, path)
		return nil
	})
	if err != nil {
		return tarResult{}, fmt.Errorf("failed to walk directory: %w", err)
	}

	sort.Strings(paths)

	// Add entries in sorted order
	for _, path := range paths {
		info, err := os.Lstat(path)
		if err != nil {
			return tarResult{}, fmt.Errorf("failed to stat %s: %w", path, err)
		}

		// Get relative path from skill directory
		relPath, err := filepath.Rel(dir, path)
		if err != nil {
			return tarResult{}, fmt.Errorf("failed to get relative path: %w", err)
		}

		// Skip the directory itself (relPath == ".")
		if relPath == "." {
			continue
		}

		// Build tar path: <skillName>/<relPath>
		tarPath := filepath.Join(skillName, relPath)
		// Normalize to forward slashes for tar
		tarPath = strings.ReplaceAll(tarPath, string(filepath.Separator), "/")

		// Create header
		header := &tar.Header{
			Name:    tarPath,
			Mode:    int64(info.Mode().Perm()),
			ModTime: fixedTime,
		}

		if info.IsDir() {
			header.Typeflag = tar.TypeDir
			header.Name += "/"
		} else if info.Mode().IsRegular() {
			header.Typeflag = tar.TypeReg
			header.Size = info.Size()
		} else {
			// Skip symlinks, devices, etc.
			continue
		}

		if err := tw.WriteHeader(header); err != nil {
			return tarResult{}, fmt.Errorf("failed to write tar header: %w", err)
		}

		// Write file content for regular files
		if info.Mode().IsRegular() {
			f, err := os.Open(path)
			if err != nil {
				return tarResult{}, fmt.Errorf("failed to open %s: %w", path, err)
			}
			if _, err := io.Copy(tw, f); err != nil {
				f.Close()
				return tarResult{}, fmt.Errorf("failed to copy file content: %w", err)
			}
			f.Close()
		}
	}

	if err := tw.Close(); err != nil {
		return tarResult{}, fmt.Errorf("failed to close tar writer: %w", err)
	}

	uncompressedDigest := digest.FromBytes(tarBuf.Bytes())

	// Gzip the tar.
	var gzBuf bytes.Buffer
	gw := gzip.NewWriter(&gzBuf)
	if _, err := gw.Write(tarBuf.Bytes()); err != nil {
		return tarResult{}, fmt.Errorf("failed to gzip: %w", err)
	}
	if err := gw.Close(); err != nil {
		return tarResult{}, fmt.Errorf("failed to close gzip writer: %w", err)
	}

	return tarResult{
		gzipped:            gzBuf.Bytes(),
		uncompressedDigest: uncompressedDigest,
	}, nil
}

// buildAnnotations creates OCI annotations from a SkillCard.
func buildAnnotations(sc card.SkillCard) map[string]string {
	annotations := map[string]string{
		AnnotationTitle:       sc.Metadata.Name,
		AnnotationVersion:     sc.Metadata.Version,
		AnnotationDescription: sc.Metadata.Description,
		AnnotationSkillName:   sc.Metadata.Name,
		AnnotationCreated:     time.Now().UTC().Format(time.RFC3339),
	}

	if sc.Metadata.License != "" {
		annotations[AnnotationLicenses] = sc.Metadata.License
	}

	if sc.Spec.Resources.EstimatedMemory != "" {
		annotations[AnnotationResourcesMemory] = sc.Spec.Resources.EstimatedMemory
	}

	if sc.Spec.Resources.EstimatedCPU != "" {
		annotations[AnnotationResourcesCPU] = sc.Spec.Resources.EstimatedCPU
	}

	if len(sc.Spec.Tools.Required) > 0 {
		annotations[AnnotationToolsRequired] = strings.Join(sc.Spec.Tools.Required, ",")
	}

	return annotations
}
