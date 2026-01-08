# Stage 1: Build
FROM --platform=$BUILDPLATFORM golang:1.25-bookworm AS builder

ARG TARGETARCH
ARG BUILDARCH

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

# Install swag for docs generation
RUN go install github.com/swaggo/swag/cmd/swag@latest

# Install tailwind CLI dynamically based on BUILD architecture (must run on build host)
RUN apt-get update && apt-get install -y curl && \
    if [ "$BUILDARCH" = "arm64" ]; then \
      curl -sLO https://github.com/tailwindlabs/tailwindcss/releases/download/v4.1.18/tailwindcss-linux-arm64; \
      mv tailwindcss-linux-arm64 tailwindcss; \
    else \
      curl -sLO https://github.com/tailwindlabs/tailwindcss/releases/download/v4.1.18/tailwindcss-linux-x64; \
      mv tailwindcss-linux-x64 tailwindcss; \
    fi && \
    chmod +x tailwindcss

COPY . .

# Generate Swagger API docs
RUN /go/bin/swag init -g main.go --output api-docs --dir ./cmd/ramble,./internal/handlers,./internal/models --parseDependency --parseInternal

# Build CSS
RUN ./tailwindcss -i ./public/css/input.css -o ./public/css/style.css --minify

RUN CGO_ENABLED=0 GOOS=linux GOARCH=$TARGETARCH go build -o ramble ./cmd/ramble

# Stage 2: Final Image
FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /root/

COPY --from=builder /app/ramble .
COPY --from=builder /app/views ./views
COPY --from=builder /app/public ./public
COPY --from=builder /app/docs ./docs

EXPOSE 3000

CMD ["./ramble", "server"]
