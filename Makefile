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

.PHONY: build test lint fmt clean image image-push configmap-gen configmap-apply

build:
	go build -o $(BINDIR)/$(BINARY) ./cmd/docsclaw

test:
	go test ./...

lint:
	golangci-lint run ./...

fmt:
	gofmt -w .

clean:
	rm -rf $(BINDIR)

image:
	$(CONTAINER_ENGINE) build \
		--platform linux/amd64 \
		-t $(REGISTRY):$(DEV_TAG) \
		-f Dockerfile .

image-push: image
	$(CONTAINER_ENGINE) push $(REGISTRY):$(DEV_TAG)

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
	@# --- Skills ConfigMap (only if skills/ exists) ---
	@if [ -d "$(CONFIG_DIR)/skills" ]; then \
		SKILL_FILES=""; \
		for skill_dir in $(CONFIG_DIR)/skills/*/; do \
			skill_name=$$(basename "$$skill_dir"); \
			[ -f "$$skill_dir/SKILL.md" ] && \
				SKILL_FILES="$$SKILL_FILES --from-file=$$skill_name.SKILL.md=$$skill_dir/SKILL.md"; \
		done; \
		$(KUBECTL) create configmap $(NAME)-skills \
			$$SKILL_FILES \
			$(if $(NAMESPACE),--namespace $(NAMESPACE),) \
			--dry-run=client -o yaml > $(OUTDIR)/skills.yaml; \
		echo "  Created $(OUTDIR)/skills.yaml"; \
	else \
		echo "  No skills/ directory found, skipping skills ConfigMap"; \
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
	@# --- Skills ConfigMap (only if skills/ exists) ---
	@if [ -d "$(CONFIG_DIR)/skills" ]; then \
		SKILL_FILES=""; \
		for skill_dir in $(CONFIG_DIR)/skills/*/; do \
			skill_name=$$(basename "$$skill_dir"); \
			[ -f "$$skill_dir/SKILL.md" ] && \
				SKILL_FILES="$$SKILL_FILES --from-file=$$skill_name.SKILL.md=$$skill_dir/SKILL.md"; \
		done; \
		$(KUBECTL) create configmap $(NAME)-skills \
			$$SKILL_FILES \
			$(if $(NAMESPACE),--namespace $(NAMESPACE),) \
			--dry-run=client -o yaml | $(KUBECTL) apply -f -; \
		echo "  Applied $(NAME)-skills"; \
	else \
		echo "  No skills/ directory found, skipping skills ConfigMap"; \
	fi
	@echo "Done."
