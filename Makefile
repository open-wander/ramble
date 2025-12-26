# Build the application
build:
	go build -o bin/rmbl cmd/server/main.go

# Run the application
run:
	go run cmd/server/main.go

# Run with hot reload (requires air)
dev:
	air

# Run migrations (handled automatically by app, but good to have)
migrate:
	go run cmd/server/main.go -migrate

# Test
test:
	go test ./...

# Generate Swagger documentation
swagger:
	go run github.com/swaggo/swag/cmd/swag@latest init -g main.go --output docs --dir ./cmd/server,./internal/handlers,./internal/models --parseDependency --parseInternal

# Bootstrap development environment
bootstrap:
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
IMAGE_NAME := haigqn.hhome:5000/rmbl

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
	./tailwindcss -i ./public/css/input.css -o ./public/css/style.css --minify

# Watch CSS for development
css-watch:
	./tailwindcss -i ./public/css/input.css -o ./public/css/style.css --watch

# Docker Build (ARM64)
docker-build:
	docker build --platform linux/arm64 -t $(IMAGE_NAME):$(VERSION) -t $(IMAGE_NAME):latest .
	sed -i '' 's|image = "$(IMAGE_NAME):.*"|image = "$(IMAGE_NAME):$(VERSION)"|' rmbl.nomad.hcl

# Docker Push
docker-push:
	docker push $(IMAGE_NAME):$(VERSION)
	docker push $(IMAGE_NAME):latest

# Combined target for convenience
deploy: docker-build docker-push
	export NOMAD_ADDR=http://nmd-svr1:4646 && nomad job run rmbl.nomad.hcl

# Set Nomad Variables from config.yml
nomad-vars:
	go run cmd/nomad-vars/main.go

.PHONY: build run dev migrate test swagger bootstrap update security docker-build docker-push nomad-vars tailwind-install css-build css-watch deploy
