# Build the application
build:
	go build -o bin/ramble ./cmd/ramble

# Run the server
run:
	go run ./cmd/ramble server

# Run with hot reload (requires air)
# Builds CSS first, then runs air and css-watch in parallel
dev: css-build
	@if [ ! -f ./tailwindcss ]; then echo "Tailwind CLI not found. Run 'make bootstrap' first."; exit 1; fi
	@trap 'kill 0' EXIT; \
	./tailwindcss -i ./public/css/input.css -o ./public/css/style.css --watch & \
	air

# Run migrations (handled automatically by app, but good to have)
migrate:
	go run ./cmd/ramble server --seed

# Test
test:
	go test ./...

# Generate Swagger documentation
swagger:
	go run github.com/swaggo/swag/cmd/swag@$(SWAG_VERSION) init -g main.go --output api-docs --dir ./cmd/ramble,./internal/handlers,./internal/models --parseDependency --parseInternal

# Static analysis tools. Pinned so a PR never fails because a tool changed
# overnight; the weekly security workflow runs both at master so new checks and
# toolchain breaks show up early - bump these when it passes. Both are master
# commits because no tagged release reads Go 1.27 export data yet.
GOSEC_VERSION ?= v2.29.1-0.20261009120814-7b1b5cebe007
STATICCHECK_VERSION ?= v0.7.0-0.dev.0.20261009230814-452d5bb86b45
ACTIONLINT_VERSION ?= v1.7.12

# Build tools. The Dockerfile declares the same versions as ARG defaults;
# check-pins fails if the two drift apart.
SWAG_VERSION ?= v1.16.6
TAILWIND_VERSION ?= v4.1.18

actionlint:
	go run github.com/rhysd/actionlint/cmd/actionlint@$(ACTIONLINT_VERSION)

check-pins:
	@for v in SWAG_VERSION TAILWIND_VERSION; do \
		want=$$($(MAKE) -s print-$$v); \
		got=$$(sed -n "s/^ARG $$v=//p" Dockerfile); \
		if [ "$$want" != "$$got" ]; then echo "$$v: Makefile $$want, Dockerfile $$got"; exit 1; fi; \
	done; echo "tool pins match"

print-%:
	@echo $($*)

staticcheck:
	go run honnef.co/go/tools/cmd/staticcheck@$(STATICCHECK_VERSION) ./...
	go run honnef.co/go/tools/cmd/staticcheck@$(STATICCHECK_VERSION) -tags=security ./...

gosec:
	go run github.com/securego/gosec/v2/cmd/gosec@$(GOSEC_VERSION) ./...

# Bootstrap development environment
bootstrap: tailwind-install
	go install github.com/air-verse/air@latest
	go install github.com/swaggo/swag/cmd/swag@$(SWAG_VERSION)
	go install github.com/securego/gosec/v2/cmd/gosec@$(GOSEC_VERSION)
	go install honnef.co/go/tools/cmd/staticcheck@$(STATICCHECK_VERSION)
	go install golang.org/x/vuln/cmd/govulncheck@latest
	go mod tidy
	git config core.hooksPath .githooks

# Update dependencies
update:
	go get -u ./...
	go mod tidy

# Versioning - the latest git tag is the source of truth
VERSION := $(shell git describe --tags --abbrev=0 2>/dev/null || echo dev)
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
BUILD_DATE := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
IMAGE_NAME := ghcr.io/open-wander/ramble
MACOS_SIGN_IDENTITY ?= Developer ID Application: Lance Norman Haig (VC27MNBWFP)
MACOS_NOTARY_PROFILE ?= VC27MNBWFP-notary

# Run security checks
security:
	@echo "Running govulncheck..."
	govulncheck ./...
	@echo "\nRunning gosec..."
	go run github.com/securego/gosec/v2/cmd/gosec@$(GOSEC_VERSION) -exclude-dir=legacy ./...

# Download Tailwind CLI (Darwin ARM64 for local dev)
tailwind-install:
	curl -sLO https://github.com/tailwindlabs/tailwindcss/releases/download/$(TAILWIND_VERSION)/tailwindcss-macos-arm64
	mv tailwindcss-macos-arm64 tailwindcss
	chmod +x tailwindcss

# Build CSS for production
css-build:
	@if [ ! -f ./tailwindcss ]; then echo "Tailwind CLI not found. Run 'make tailwind-install' first."; exit 1; fi
	./tailwindcss -i ./public/css/input.css -o ./public/css/style.css --minify

# Watch CSS for development
css-watch:
	./tailwindcss -i ./public/css/input.css -o ./public/css/style.css --watch

# Docker Build (ARM64)
docker-build:
	docker build --platform linux/arm64 \
		--build-arg VERSION=$(VERSION) \
		--build-arg COMMIT=$(COMMIT) \
		--build-arg BUILD_DATE=$(BUILD_DATE) \
		-t $(IMAGE_NAME):$(VERSION) -t $(IMAGE_NAME):latest .
	sed -i '' 's|image = "$(IMAGE_NAME):.*"|image = "$(IMAGE_NAME):$(VERSION)"|' ramble.nomad.hcl

# Docker Push
docker-push:
	docker push $(IMAGE_NAME):$(VERSION)
	docker push $(IMAGE_NAME):latest

# Combined target for convenience
deploy: docker-build docker-push
	export NOMAD_ADDR=http://nomad-server:4646 && nomad job run ramble.nomad.hcl

# Set Nomad Variables from config.yml
nomad-vars:
	go run cmd/nomad-vars/main.go

# Release helpers - trigger GitHub Actions release workflow
release-patch:
	@echo "Triggering patch release via GitHub Actions..."
	gh workflow run release.yml -f bump_type=patch -f deploy_compose=true -f deploy_nomad=false
	@echo "Release workflow triggered. Check progress at: https://github.com/open-wander/ramble/actions"

release-minor:
	@echo "Triggering minor release via GitHub Actions..."
	gh workflow run release.yml -f bump_type=minor -f deploy_compose=true -f deploy_nomad=false
	@echo "Release workflow triggered. Check progress at: https://github.com/open-wander/ramble/actions"

release-major:
	@echo "Triggering major release via GitHub Actions..."
	gh workflow run release.yml -f bump_type=major -f deploy_compose=true -f deploy_nomad=false
	@echo "Release workflow triggered. Check progress at: https://github.com/open-wander/ramble/actions"

# Publish signed and notarized CLI artifacts after the release workflow creates
# the tag. This must run on the Mac that holds the Developer ID identity and
# notarytool Keychain profile.
release-cli:
	@MACOS_SIGN_IDENTITY="$(MACOS_SIGN_IDENTITY)" \
		MACOS_NOTARY_PROFILE="$(MACOS_NOTARY_PROFILE)" \
		./scripts/release-cli-local.sh

# Interactive release - prompts for bump type
release:
	@echo "Select version bump type:"
	@echo "  1) patch ($(VERSION) -> next patch)"
	@echo "  2) minor ($(VERSION) -> next minor)"
	@echo "  3) major ($(VERSION) -> next major)"
	@read -p "Enter choice [1-3]: " choice; \
	case $$choice in \
		1) $(MAKE) release-patch ;; \
		2) $(MAKE) release-minor ;; \
		3) $(MAKE) release-major ;; \
		*) echo "Invalid choice" ;; \
	esac

.PHONY: build run dev migrate test swagger bootstrap update security staticcheck gosec actionlint check-pins docker-build docker-push nomad-vars tailwind-install css-build css-watch deploy release release-patch release-minor release-major release-cli
