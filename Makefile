BINARY := docsclaw
BINDIR := bin

.PHONY: build test lint fmt clean

build:
	go build -o $(BINDIR)/$(BINARY) ./cmd/docsclaw

test:
	go test ./... -v

lint:
	golangci-lint run ./...

fmt:
	gofmt -w .

clean:
	rm -rf $(BINDIR)
