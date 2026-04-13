package oci

import (
	"context"
	"encoding/json"
	"fmt"

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

	// 5. Find the SkillCard layer (CardMediaType)
	var cardDesc *ocispec.Descriptor
	for i := range manifest.Layers {
		if manifest.Layers[i].MediaType == CardMediaType {
			cardDesc = &manifest.Layers[i]
			break
		}
	}

	if cardDesc == nil {
		return card.SkillCard{}, fmt.Errorf("skill card layer not found in manifest")
	}

	// 6. Fetch the SkillCard layer
	cardData, err := fetchBlob(ctx, localStore, *cardDesc)
	if err != nil {
		return card.SkillCard{}, fmt.Errorf("failed to fetch skill card layer: %w", err)
	}

	// 7. Unmarshal as YAML into card.SkillCard
	var sc card.SkillCard
	if err := yaml.Unmarshal(cardData, &sc); err != nil {
		return card.SkillCard{}, fmt.Errorf("failed to unmarshal skill card: %w", err)
	}

	return sc, nil
}
