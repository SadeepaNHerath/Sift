BINARY_NAME := sf
BIN_DIR := bin
CMD_DIR := ./cmd/sf

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

LDFLAGS := -s -w -X main.version=$(VERSION)

INSTALL_PATH ?= /usr/local/bin

.PHONY: all build test lint install clean fmt vet help

all: fmt vet test build

build:
	@mkdir -p $(BIN_DIR)
	CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/$(BINARY_NAME) $(CMD_DIR)

test:
	go test -v -race ./...

lint:
	golangci-lint run

install: build
	@echo "Installing $(BINARY_NAME) to $(INSTALL_PATH)..."
	@cp $(BIN_DIR)/$(BINARY_NAME) $(INSTALL_PATH)/$(BINARY_NAME) || sudo cp $(BIN_DIR)/$(BINARY_NAME) $(INSTALL_PATH)/$(BINARY_NAME)
	@chmod 755 $(INSTALL_PATH)/$(BINARY_NAME)
	@echo "Successfully installed $(BINARY_NAME) to $(INSTALL_PATH)/$(BINARY_NAME)"

clean:
	rm -rf $(BIN_DIR)

fmt:
	gofmt -w .

vet:
	go vet ./...

help:
	@echo "Sift (sf) Build System"
	@echo ""
	@echo "Targets:"
	@echo "  build    - Compile binary with CGO_ENABLED=0 and version ldflags"
	@echo "  test     - Run unit tests with race detection enabled"
	@echo "  lint     - Run golangci-lint on the repository"
	@echo "  install  - Compile and install sf to $(INSTALL_PATH)"
	@echo "  clean    - Remove compiled binaries and build artifacts"
	@echo "  fmt      - Format all Go source files with gofmt"
	@echo "  vet      - Analyze source code for common errors with go vet"
	@echo "  all      - Execute fmt, vet, test, and build in sequence"
