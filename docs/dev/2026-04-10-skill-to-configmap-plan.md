# Skill-to-ConfigMap Tooling Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use
> superpowers:subagent-driven-development (recommended) or
> superpowers:executing-plans to implement this plan task-by-task.
> Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add Makefile targets that generate Kubernetes ConfigMap YAML
files from an agent config directory and optionally apply them to a
cluster.

**Architecture:** Two shell-based Makefile targets (`configmap-gen`,
`configmap-apply`) that use `kubectl`/`oc` with `--from-file` and
`--dry-run=client -o yaml` to produce idempotent ConfigMap manifests.
No Go code needed.

**Tech Stack:** Make, kubectl/oc CLI

---

## File structure

| File | Action | Responsibility |
|------|--------|----------------|
| `Makefile` | Modify | Add `configmap-gen` and `configmap-apply` targets |
| `.gitignore` | Modify | Add `deploy/generated/` |

---

### Task 1: Add `configmap-gen` target to Makefile

**Files:**
- Modify: `Makefile`

- [ ] **Step 1: Add variables to Makefile**

Add these lines after the existing `CONTAINER_ENGINE` / `PODMAN_CONNECTION`
block (after line 12):

```makefile
# ConfigMap generation
CONFIG_DIR ?=
NAME ?= $(notdir $(patsubst %/,%,$(CONFIG_DIR)))
OUTDIR ?= deploy/generated/$(NAME)
NAMESPACE ?=
KUBECTL ?= oc
```

- [ ] **Step 2: Add the `configmap-gen` target**

Add after the `image-push` target. The target generates two YAML files:
`config.yaml` (agent config files) and `skills.yaml` (all skills).

```makefile
# --- ConfigMap generation ---

.PHONY: configmap-gen configmap-apply

configmap-gen:
ifndef CONFIG_DIR
	$(error CONFIG_DIR is required. Usage: make configmap-gen CONFIG_DIR=testdata/standalone)
endif
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
				SKILL_FILES="$$SKILL_FILES --from-file=$$skill_name/SKILL.md=$$skill_dir/SKILL.md"; \
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
```

- [ ] **Step 3: Update the `.PHONY` declaration**

Change line 14 from:

```makefile
.PHONY: build test lint fmt clean image image-push
```

to:

```makefile
.PHONY: build test lint fmt clean image image-push configmap-gen configmap-apply
```

- [ ] **Step 4: Test `configmap-gen` with `testdata/standalone`**

Run:

```bash
make configmap-gen CONFIG_DIR=testdata/standalone
```

Expected output:

```
Generating ConfigMap YAML files in deploy/generated/standalone/...
  Created deploy/generated/standalone/config.yaml
  Created deploy/generated/standalone/skills.yaml
Done. Apply with: oc apply -f deploy/generated/standalone/
```

Verify the generated files:

```bash
cat deploy/generated/standalone/config.yaml
```

Expected: a valid ConfigMap YAML with `name: standalone-config` containing
keys `system-prompt.txt`, `agent-card.json`, `agent-config.yaml`.

```bash
cat deploy/generated/standalone/skills.yaml
```

Expected: a valid ConfigMap YAML with `name: standalone-skills` containing
keys `code-review/SKILL.md` and `url-summary/SKILL.md`.

- [ ] **Step 5: Test with custom NAME**

Run:

```bash
make configmap-gen CONFIG_DIR=testdata/standalone NAME=my-agent OUTDIR=deploy/generated/my-agent
```

Expected: files in `deploy/generated/my-agent/` with ConfigMap names
`my-agent-config` and `my-agent-skills`.

- [ ] **Step 6: Test with a config directory that has no skills**

Run:

```bash
make configmap-gen CONFIG_DIR=testdata/summarizer-hr
```

Expected output:

```
Generating ConfigMap YAML files in deploy/generated/summarizer-hr/...
  Created deploy/generated/summarizer-hr/config.yaml
  No skills/ directory found, skipping skills ConfigMap
Done. Apply with: oc apply -f deploy/generated/summarizer-hr/
```

- [ ] **Step 7: Commit**

```bash
git add Makefile
git commit -s -m "feat: add configmap-gen Makefile target (#6)"
```

---

### Task 2: Add `configmap-apply` target

**Files:**
- Modify: `Makefile`

- [ ] **Step 1: Add the `configmap-apply` target**

Add immediately after the `configmap-gen` target:

```makefile
configmap-apply:
ifndef CONFIG_DIR
	$(error CONFIG_DIR is required. Usage: make configmap-apply CONFIG_DIR=testdata/standalone)
endif
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
				SKILL_FILES="$$SKILL_FILES --from-file=$$skill_name/SKILL.md=$$skill_dir/SKILL.md"; \
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
```

- [ ] **Step 2: Test `configmap-apply` with dry-run**

If you don't have a cluster available, verify the command generation
works by checking the `configmap-gen` output (which uses the same
logic). If a cluster is available:

```bash
make configmap-apply CONFIG_DIR=testdata/standalone NAMESPACE=docsclaw
```

Expected:

```
Applying ConfigMaps from testdata/standalone...
  Applied standalone-config
  Applied standalone-skills
Done.
```

- [ ] **Step 3: Commit**

```bash
git add Makefile
git commit -s -m "feat: add configmap-apply Makefile target (#6)"
```

---

### Task 3: Add `deploy/generated/` to `.gitignore`

**Files:**
- Modify: `.gitignore`

- [ ] **Step 1: Add the gitignore entry**

Add to the end of `.gitignore`:

```gitignore

# Generated ConfigMap YAML files
deploy/generated/
```

- [ ] **Step 2: Clean up any generated files from testing**

```bash
rm -rf deploy/generated/
```

- [ ] **Step 3: Verify gitignore works**

```bash
make configmap-gen CONFIG_DIR=testdata/standalone
git status
```

Expected: `deploy/generated/` should NOT appear in `git status` output.
Only `.gitignore` should show as modified.

- [ ] **Step 4: Commit**

```bash
git add .gitignore
git commit -s -m "chore: gitignore generated ConfigMap YAML files (#6)"
```
