.PHONY: clean lint test build \
		build-linux-arm64 build-linux-amd64 multi-arch-image-%

export GO111MODULE=on

BIN_NAME := ingress-nginx-analyzer

VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_DATE := $(shell date -u '+%Y-%m-%d_%I:%M:%S%p')

# Default build target
GOOS := $(shell go env GOOS)
GOARCH := $(shell go env GOARCH)
DOCKER_BUILD_PLATFORMS ?= linux/amd64,linux/arm64

default: lint test clean build

build:
	@echo VERSION: $(VERSION) BUILD_DATE: $(BUILD_DATE)
	CGO_ENABLED=0 GOOS=${GOOS} GOARCH=${GOARCH} go build \
	-trimpath \
	-ldflags "-s -w \
		-X github.com/traefik/ingress-nginx-analyzer/pkg/version.Version=$(VERSION) \
		-X github.com/traefik/ingress-nginx-analyzer/pkg/version.BuildDate=$(BUILD_DATE) \
		-X github.com/traefik/ingress-nginx-analyzer/pkg/client.Token=$(AUTH_TOKEN) \
		-X github.com/traefik/ingress-nginx-analyzer/pkg/client.ClientCertB64=$(MTLS_CLIENT_CERT_B64) \
		-X github.com/traefik/ingress-nginx-analyzer/pkg/client.ClientKeyB64=$(MTLS_CLIENT_KEY_B64) \
		-X github.com/traefik/ingress-nginx-analyzer/pkg/client.CACertB64=$(MTLS_CA_CERT_B64)" \
	-o "./dist/${GOOS}/${GOARCH}/${BIN_NAME}" ./cmd

clean:
	rm -rf dist/ cover.out

lint:
	golangci-lint run --timeout 2m

test:
	go test -v -cover ./...

dist:
	mkdir dist

build-linux-arm64: export GOOS := linux
build-linux-arm64: export GOARCH := arm64
build-linux-arm64:
	make build

build-linux-amd64: export GOOS := linux
build-linux-amd64: export GOARCH := amd64
build-linux-amd64:
	make build

## Build Multi archs Docker image
multi-arch-image-%: build-linux-amd64 build-linux-arm64
	docker buildx build $(DOCKER_BUILDX_ARGS) -t gcr.io/traefiklabs/$(BIN_NAME):$* --platform=$(DOCKER_BUILD_PLATFORMS) -f Dockerfile .
