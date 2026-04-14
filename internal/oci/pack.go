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

	// 2. Build config blob
	cfg := buildConfig(sc)
	configData, err := json.Marshal(cfg)
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("failed to marshal config: %w", err)
	}

	// Choose config media type based on AsImage option
	configMediaType := ConfigMediaType
	if opts.AsImage {
		configMediaType = ImageConfigMediaType
	}

	configDesc, err := pushBlob(ctx, target, configMediaType, configData)
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("failed to push config blob: %w", err)
	}

	// 3. Create layer 0: skill.yaml as CardMediaType
	skillYAMLData, err := os.ReadFile(skillYAMLPath)
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("failed to read skill.yaml: %w", err)
	}

	cardDesc, err := pushBlob(ctx, target, CardMediaType, skillYAMLData)
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("failed to push card layer: %w", err)
	}

	// 4. Create layer 1: tar+gzip of entire skill directory
	tarData, err := tarDirectory(skillDir, sc.Metadata.Name)
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("failed to create tarball: %w", err)
	}

	contentDesc, err := pushBlob(ctx, target, ContentMediaType, tarData)
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("failed to push content layer: %w", err)
	}

	// 5. Build manifest with annotations
	annotations := buildAnnotations(sc)

	manifest := ocispec.Manifest{
		Versioned: specs.Versioned{SchemaVersion: 2},
		MediaType: ocispec.MediaTypeImageManifest,
		Config:    configDesc,
		Layers: []ocispec.Descriptor{
			cardDesc,
			contentDesc,
		},
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

// tarDirectory creates a deterministic tar+gzip of the skill directory.
// All entries are rooted at <skillName>/ and have a fixed mtime for reproducibility.
func tarDirectory(dir, skillName string) ([]byte, error) {
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

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
		return nil, fmt.Errorf("failed to walk directory: %w", err)
	}

	sort.Strings(paths)

	// Add entries in sorted order
	for _, path := range paths {
		info, err := os.Lstat(path)
		if err != nil {
			return nil, fmt.Errorf("failed to stat %s: %w", path, err)
		}

		// Get relative path from skill directory
		relPath, err := filepath.Rel(dir, path)
		if err != nil {
			return nil, fmt.Errorf("failed to get relative path: %w", err)
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
			return nil, fmt.Errorf("failed to write tar header: %w", err)
		}

		// Write file content for regular files
		if info.Mode().IsRegular() {
			f, err := os.Open(path)
			if err != nil {
				return nil, fmt.Errorf("failed to open %s: %w", path, err)
			}
			if _, err := io.Copy(tw, f); err != nil {
				f.Close()
				return nil, fmt.Errorf("failed to copy file content: %w", err)
			}
			f.Close()
		}
	}

	if err := tw.Close(); err != nil {
		return nil, fmt.Errorf("failed to close tar writer: %w", err)
	}
	if err := gw.Close(); err != nil {
		return nil, fmt.Errorf("failed to close gzip writer: %w", err)
	}

	return buf.Bytes(), nil
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
