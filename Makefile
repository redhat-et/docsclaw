BINARY := docsclaw
BINDIR := bin
REGISTRY ?= ghcr.io/redhat-et/docsclaw
GIT_SHA := $(shell git rev-parse --short HEAD)
DEV_TAG ?= $(GIT_SHA)
CONTAINER_ENGINE ?= podman
# Remote Podman connection (e.g., PODMAN_CONNECTION=rhel)
# When set, builds run on the remote host via 'podman --connection <name>'
PODMAN_CONNECTION ?=
ifdef PODMAN_CONNECTION
  CONTAINER_ENGINE := podman --connection $(PODMAN_CONNECTION)
endif

# ConfigMap generation
CONFIG_DIR ?=
NAME ?= $(notdir $(patsubst %/,%,$(CONFIG_DIR)))
OUTDIR ?= deploy/generated/$(NAME)
NAMESPACE ?=
KUBECTL ?= oc

.PHONY: build build-skill-puller test lint fmt clean image image-push image-security image-security-push agent-build agent-image agent-push configmap-gen configmap-apply

build:
	go build -o $(BINDIR)/$(BINARY) ./cmd/docsclaw

build-skill-puller:
	go build -o $(BINDIR)/skill-puller ./cmd/skill-puller

test:
	go test ./...

lint:
	golangci-lint run ./...

fmt:
	gofmt -w .

clean:
	rm -rf $(BINDIR) build/

image:
	$(CONTAINER_ENGINE) build \
		--platform linux/amd64 \
		-t $(REGISTRY):$(DEV_TAG) \
		-f Dockerfile .

image-push: image
	$(CONTAINER_ENGINE) push $(REGISTRY):$(DEV_TAG)

image-security:
	$(CONTAINER_ENGINE) build \
		--platform linux/amd64 \
		-t $(REGISTRY):$(DEV_TAG)-security \
		-f containers/Containerfile.security .

image-security-push: image-security
	$(CONTAINER_ENGINE) push $(REGISTRY):$(DEV_TAG)-security

# --- Agent image from manifest ---
# Build a custom agent image from an agent manifest.
# The manifest declares which OS tools to install (curl, jq, git, etc.).
#
# Usage:
#   make agent-build MANIFEST=testdata/manifest/nps-agent.yaml
#   make agent-image MANIFEST=testdata/manifest/nps-agent.yaml TAG=ghcr.io/org/nps-agent:1.0.0
#   make agent-push  MANIFEST=testdata/manifest/nps-agent.yaml TAG=ghcr.io/org/nps-agent:1.0.0

MANIFEST ?=
AGENT_BUILDDIR ?= build/agent
TAG ?= $(REGISTRY):$(DEV_TAG)

agent-build: build
ifndef MANIFEST
	$(error MANIFEST is required. Usage: make agent-build MANIFEST=path/to/agent-manifest.yaml)
endif
	@mkdir -p $(AGENT_BUILDDIR)
	@echo "==> Generating build artifacts from $(MANIFEST)..."
	$(BINDIR)/$(BINARY) build --manifest $(MANIFEST) --output $(AGENT_BUILDDIR)
	@cp $(AGENT_BUILDDIR)/Containerfile Containerfile.agent
	@cp $(AGENT_BUILDDIR)/tools.json tools.json
	@echo "==> Build context ready (Containerfile.agent + tools.json)"

agent-image: agent-build
	@echo "==> Building container image $(TAG)..."
	$(CONTAINER_ENGINE) build \
		--platform linux/amd64 \
		-t $(TAG) \
		-f Containerfile.agent .
	@rm -f Containerfile.agent tools.json
	@echo "==> Image built: $(TAG)"

agent-push: agent-image
	$(CONTAINER_ENGINE) push $(TAG)

# --- ConfigMap generation ---

configmap-gen:
ifndef CONFIG_DIR
	$(error CONFIG_DIR is required. Usage: make configmap-gen CONFIG_DIR=testdata/standalone)
endif
	@test -d "$(CONFIG_DIR)" || { echo "ERROR: $(CONFIG_DIR) does not exist"; exit 1; }
	@test -f "$(CONFIG_DIR)/system-prompt.txt" || { echo "ERROR: $(CONFIG_DIR)/system-prompt.txt is required"; exit 1; }
	@mkdir -p $(OUTDIR)
	@echo "Generating ConfigMap YAML files in $(OUTDIR)..."
	@# --- Agent config ConfigMap ---
	@CONFIG_FILES=""; \
	for f in system-prompt.txt agent-card.json agent-config.yaml prompts.json; do \
		[ -f "$(CONFIG_DIR)/$$f" ] && CONFIG_FILES="$$CONFIG_FILES --from-file=$$f=$(CONFIG_DIR)/$$f"; \
	done; \
	$(KUBECTL) create configmap $(NAME)-config \
		$$CONFIG_FILES \
		$(if $(NAMESPACE),--namespace $(NAMESPACE),) \
		--dry-run=client -o yaml > $(OUTDIR)/config.yaml
	@echo "  Created $(OUTDIR)/config.yaml"
	@# --- Per-skill ConfigMaps (only if skills/ exists) ---
	@if [ -d "$(CONFIG_DIR)/skills" ]; then \
		FOUND=0; \
		for skill_dir in $(CONFIG_DIR)/skills/*/; do \
			skill_name=$$(basename "$$skill_dir"); \
			[ -f "$$skill_dir/SKILL.md" ] || continue; \
			FOUND=1; \
			$(KUBECTL) create configmap $(NAME)-skill-$$skill_name \
				--from-file=SKILL.md=$$skill_dir/SKILL.md \
				$(if $(NAMESPACE),--namespace $(NAMESPACE),) \
				--dry-run=client -o yaml > $(OUTDIR)/skill-$$skill_name.yaml; \
			echo "  Created $(OUTDIR)/skill-$$skill_name.yaml"; \
		done; \
		if [ "$$FOUND" = "0" ]; then \
			echo "  No SKILL.md files found in $(CONFIG_DIR)/skills/, skipping skill ConfigMaps"; \
		fi; \
	else \
		echo "  No skills/ directory found, skipping skill ConfigMaps"; \
	fi
	@echo "Done. Apply with: $(KUBECTL) apply -f $(OUTDIR)/"

configmap-apply:
ifndef CONFIG_DIR
	$(error CONFIG_DIR is required. Usage: make configmap-apply CONFIG_DIR=testdata/standalone)
endif
	@test -d "$(CONFIG_DIR)" || { echo "ERROR: $(CONFIG_DIR) does not exist"; exit 1; }
	@test -f "$(CONFIG_DIR)/system-prompt.txt" || { echo "ERROR: $(CONFIG_DIR)/system-prompt.txt is required"; exit 1; }
	@echo "Applying ConfigMaps from $(CONFIG_DIR)..."
	@# --- Agent config ConfigMap ---
	@CONFIG_FILES=""; \
	for f in system-prompt.txt agent-card.json agent-config.yaml prompts.json; do \
		[ -f "$(CONFIG_DIR)/$$f" ] && CONFIG_FILES="$$CONFIG_FILES --from-file=$$f=$(CONFIG_DIR)/$$f"; \
	done; \
	$(KUBECTL) create configmap $(NAME)-config \
		$$CONFIG_FILES \
		$(if $(NAMESPACE),--namespace $(NAMESPACE),) \
		--dry-run=client -o yaml | $(KUBECTL) apply -f -
	@echo "  Applied $(NAME)-config"
	@# --- Per-skill ConfigMaps (only if skills/ exists) ---
	@if [ -d "$(CONFIG_DIR)/skills" ]; then \
		FOUND=0; \
		for skill_dir in $(CONFIG_DIR)/skills/*/; do \
			skill_name=$$(basename "$$skill_dir"); \
			[ -f "$$skill_dir/SKILL.md" ] || continue; \
			FOUND=1; \
			$(KUBECTL) create configmap $(NAME)-skill-$$skill_name \
				--from-file=SKILL.md=$$skill_dir/SKILL.md \
				$(if $(NAMESPACE),--namespace $(NAMESPACE),) \
				--dry-run=client -o yaml | $(KUBECTL) apply -f -; \
			echo "  Applied $(NAME)-skill-$$skill_name"; \
		done; \
		if [ "$$FOUND" = "0" ]; then \
			echo "  No SKILL.md files found in $(CONFIG_DIR)/skills/, skipping skill ConfigMaps"; \
		fi; \
	else \
		echo "  No skills/ directory found, skipping skill ConfigMaps"; \
	fi
	@echo "Done."
