package oci

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/redhat-et/docsclaw/pkg/skills/card"
	"gopkg.in/yaml.v3"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content/memory"
	"oras.land/oras-go/v2/registry"
)

// InspectOptions configures the inspect operation.
type InspectOptions struct {
	TLSVerify *bool // nil = default (true), false = plain HTTP
	Registry  oras.Target
}

// Inspect fetches only the SkillCard layer from a registry without pulling the full content.
func Inspect(ctx context.Context, ref string, opts InspectOptions) (card.SkillCard, error) {
	// 1. Resolve target registry
	target, err := resolveTarget(ref, opts.Registry, opts.TLSVerify)
	if err != nil {
		return card.SkillCard{}, fmt.Errorf("failed to resolve target: %w", err)
	}

	// 2. Parse reference
	parsedRef, err := registry.ParseReference(ref)
	if err != nil {
		return card.SkillCard{}, fmt.Errorf("failed to parse reference: %w", err)
	}

	// 3. Copy manifest to local store
	localStore := memory.New()
	copyOpts := oras.DefaultCopyOptions
	copyOpts.Concurrency = 1
	desc, err := oras.Copy(ctx, target, parsedRef.Reference, localStore, parsedRef.Reference, copyOpts)
	if err != nil {
		return card.SkillCard{}, fmt.Errorf("failed to copy from registry: %w", err)
	}

	// 4. Fetch and parse the manifest
	manifestData, err := fetchBlob(ctx, localStore, desc)
	if err != nil {
		return card.SkillCard{}, fmt.Errorf("failed to fetch manifest: %w", err)
	}

	var manifest ocispec.Manifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		return card.SkillCard{}, fmt.Errorf("failed to unmarshal manifest: %w", err)
	}

	// 5. Find the SkillCard layer (CardMediaType).
	// For image-mode artifacts, fall back to extracting skill.yaml
	// from the content layer (tar+gzip).
	for i := range manifest.Layers {
		if manifest.Layers[i].MediaType == CardMediaType {
			cardData, err := fetchBlob(ctx, localStore, manifest.Layers[i])
			if err != nil {
				return card.SkillCard{}, fmt.Errorf("failed to fetch skill card layer: %w", err)
			}
			var sc card.SkillCard
			if err := yaml.Unmarshal(cardData, &sc); err != nil {
				return card.SkillCard{}, fmt.Errorf("failed to unmarshal skill card: %w", err)
			}
			return sc, nil
		}
	}

	// Fallback: image-mode artifact — extract skill.yaml from the content layer.
	for i := range manifest.Layers {
		mt := manifest.Layers[i].MediaType
		if mt == ContentMediaType || mt == ocispec.MediaTypeImageLayerGzip {
			layerData, err := fetchBlob(ctx, localStore, manifest.Layers[i])
			if err != nil {
				return card.SkillCard{}, fmt.Errorf("failed to fetch content layer: %w", err)
			}
			return extractSkillCardFromTar(layerData)
		}
	}

	return card.SkillCard{}, fmt.Errorf("no skill card or content layer found in manifest")
}

// extractSkillCardFromTar finds and parses skill.yaml inside a tar+gzip archive.
func extractSkillCardFromTar(data []byte) (card.SkillCard, error) {
	gr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return card.SkillCard{}, fmt.Errorf("failed to decompress content layer: %w", err)
	}
	defer gr.Close()

	tr := tar.NewReader(gr)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return card.SkillCard{}, fmt.Errorf("failed to read tar: %w", err)
		}
		// Match skill.yaml at root or single-dir depth only
		// (e.g., "skill.yaml" or "resume-screener/skill.yaml",
		// but not "examples/foo/skill.yaml").
		if header.Typeflag == tar.TypeReg && isSkillYAMLPath(header.Name) {
			cardData, err := io.ReadAll(tr)
			if err != nil {
				return card.SkillCard{}, fmt.Errorf("failed to read skill.yaml from archive: %w", err)
			}
			var sc card.SkillCard
			if err := yaml.Unmarshal(cardData, &sc); err != nil {
				return card.SkillCard{}, fmt.Errorf("failed to parse skill.yaml from archive: %w", err)
			}
			return sc, nil
		}
	}
	return card.SkillCard{}, fmt.Errorf("skill.yaml not found in content layer")
}

// isSkillYAMLPath returns true if the tar entry path is "skill.yaml" or
// "<single-dir>/skill.yaml" (at most one directory level).
func isSkillYAMLPath(name string) bool {
	if name == "skill.yaml" {
		return true
	}
	// Accept "<dir>/skill.yaml" but not "a/b/skill.yaml".
	if strings.HasSuffix(name, "/skill.yaml") {
		prefix := strings.TrimSuffix(name, "/skill.yaml")
		return !strings.Contains(prefix, "/")
	}
	return false
}
