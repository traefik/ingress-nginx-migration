.PHONY: clean lint test build \
		build-linux-arm64 build-linux-amd64 multi-arch-image-%

export GO111MODULE=on

BIN_NAME := ingress-nginx-analyzer

TAG_NAME := $(shell git tag -l --contains HEAD)
SHA := $(shell git rev-parse HEAD)
BUILD_DATE := $(shell date -u '+%Y-%m-%d_%I:%M:%S%p')

# Default build target
GOOS := $(shell go env GOOS)
GOARCH := $(shell go env GOARCH)
DOCKER_BUILD_PLATFORMS ?= linux/amd64,linux/arm64

default: lint test clean build

build:
	@echo SHA: $(SHA) $(BUILD_DATE)
	CGO_ENABLED=0 GOOS=${GOOS} GOARCH=${GOARCH} go build \
	-ldflags "-s -w -X github.com/traefik/ingress-nginx-analyzer/pkg/client.Token=$(TOKEN) -X github.com/traefik/ingress-nginx-analyzer/pkg/version.Codename=$(CODENAME) -X github.com/traefik/ingress-nginx-analyzer/pkg/version.BuildDate=$(DATE)" \
	-installsuffix nocgo -o "./dist/${GOOS}/${GOARCH}/${BIN_NAME}" ./cmd

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
