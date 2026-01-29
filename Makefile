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
	go run github.com/swaggo/swag/cmd/swag@latest init -g main.go --output api-docs --dir ./cmd/ramble,./internal/handlers,./internal/models --parseDependency --parseInternal

# Bootstrap development environment
bootstrap: tailwind-install
	go install github.com/air-verse/air@latest
	go install github.com/swaggo/swag/cmd/swag@latest
	go install github.com/securego/gosec/v2/cmd/gosec@latest
	go install golang.org/x/vuln/cmd/govulncheck@latest
	go mod tidy

# Update dependencies
update:
	go get -u ./...
	go mod tidy

# Versioning
VERSION := $(shell cat VERSION)
IMAGE_NAME := ghcr.io/open-wander/ramble

# Run security checks
security:
	@echo "Running govulncheck..."
	govulncheck ./...
	@echo "\nRunning gosec..."
	gosec -exclude-dir=legacy ./...

# Download Tailwind CLI (Darwin ARM64 for local dev)
tailwind-install:
	curl -sLO https://github.com/tailwindlabs/tailwindcss/releases/latest/download/tailwindcss-macos-arm64
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
	docker build --platform linux/arm64 -t $(IMAGE_NAME):$(VERSION) -t $(IMAGE_NAME):latest .
	sed -i '' 's|image = "$(IMAGE_NAME):.*"|image = "$(IMAGE_NAME):$(VERSION)"|' ramble.nomad.hcl

# Docker Push
docker-push:
	docker push $(IMAGE_NAME):$(VERSION)
	docker push $(IMAGE_NAME):latest

# Combined target for convenience
deploy: docker-build docker-push
	export NOMAD_ADDR=http://nmd-svr1:4646 && nomad job run ramble.nomad.hcl

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

# Interactive release - prompts for bump type
release:
	@echo "Select version bump type:"
	@echo "  1) patch ($(shell cat VERSION) -> next patch)"
	@echo "  2) minor ($(shell cat VERSION) -> next minor)"
	@echo "  3) major ($(shell cat VERSION) -> next major)"
	@read -p "Enter choice [1-3]: " choice; \
	case $$choice in \
		1) $(MAKE) release-patch ;; \
		2) $(MAKE) release-minor ;; \
		3) $(MAKE) release-major ;; \
		*) echo "Invalid choice" ;; \
	esac

.PHONY: build run dev migrate test swagger bootstrap update security docker-build docker-push nomad-vars tailwind-install css-build css-watch deploy release release-patch release-minor release-major
