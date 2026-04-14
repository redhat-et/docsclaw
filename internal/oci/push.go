package oci

import (
	"context"
	"fmt"

	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content/memory"
	"oras.land/oras-go/v2/registry"
	"oras.land/oras-go/v2/registry/remote"
	"oras.land/oras-go/v2/registry/remote/auth"
	"oras.land/oras-go/v2/registry/remote/credentials"
)

// PushOptions configures the push operation.
type PushOptions struct {
	AsImage      bool
	TLSVerify    *bool // nil = default (true), false = plain HTTP
	// Registry overrides the default remote registry (for testing with memory store).
	Registry oras.Target
}

// Push packs a skill directory and pushes it to an OCI registry.
func Push(ctx context.Context, skillDir, ref string, opts PushOptions) error {
	// 1. Create a local memory store
	localStore := memory.New()

	// 2. Pack the skill into the local store
	packOpts := PackOptions{AsImage: opts.AsImage}
	desc, err := Pack(ctx, skillDir, localStore, packOpts)
	if err != nil {
		return fmt.Errorf("failed to pack skill: %w", err)
	}

	// 3. Resolve the target registry
	target, err := resolveTarget(ref, opts.Registry, opts.TLSVerify)
	if err != nil {
		return fmt.Errorf("failed to resolve target: %w", err)
	}

	// 4. Parse the reference to get the tag
	parsedRef, err := registry.ParseReference(ref)
	if err != nil {
		return fmt.Errorf("failed to parse reference: %w", err)
	}

	// 5. Tag the manifest in the local store
	if err := localStore.Tag(ctx, desc, parsedRef.Reference); err != nil {
		return fmt.Errorf("failed to tag manifest: %w", err)
	}

	// 6. Copy from local store to target
	copyOpts := oras.DefaultCopyOptions
	copyOpts.Concurrency = 1 // Sequential copy to avoid race conditions
	_, err = oras.Copy(ctx, localStore, parsedRef.Reference, target, parsedRef.Reference, copyOpts)
	if err != nil {
		return fmt.Errorf("failed to copy to registry: %w", err)
	}

	return nil
}

// resolveTarget returns the target registry. If override is set, it is used;
// otherwise, a remote repository is created from the ref.
// tlsVerify: nil = default (HTTPS), ptr to false = plain HTTP.
func resolveTarget(ref string, override oras.Target, tlsVerify *bool) (oras.Target, error) {
	if override != nil {
		return override, nil
	}

	parsedRef, err := registry.ParseReference(ref)
	if err != nil {
		return nil, fmt.Errorf("failed to parse reference: %w", err)
	}

	repo, err := remote.NewRepository(parsedRef.Registry + "/" + parsedRef.Repository)
	if err != nil {
		return nil, fmt.Errorf("failed to create remote repository: %w", err)
	}

	if tlsVerify != nil && !*tlsVerify {
		repo.PlainHTTP = true
	}

	// Set up credential resolution from Docker/Podman config.
	credStore, err := credentials.NewStoreFromDocker(credentials.StoreOptions{})
	if err == nil {
		repo.Client = &auth.Client{
			Credential: credentials.Credential(credStore),
		}
	}

	return repo, nil
}
