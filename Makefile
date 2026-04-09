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

.PHONY: build test lint fmt clean image image-push

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
