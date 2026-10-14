# Stage 1: Build
FROM --platform=$BUILDPLATFORM golang:1.27-bookworm AS builder

ARG TARGETARCH
ARG BUILDARCH

# Version metadata injected via ldflags (set by the release workflow).
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

# Tool versions: keep in step with SWAG_VERSION and TAILWIND_VERSION in the Makefile
# (make check-pins, run in CI, fails if they differ).
ARG SWAG_VERSION=v1.16.6
ARG TAILWIND_VERSION=v4.1.18

# Install swag for docs generation
RUN go install github.com/swaggo/swag/cmd/swag@${SWAG_VERSION}

# Install tailwind CLI dynamically based on BUILD architecture (must run on build host)
RUN apt-get update && apt-get install -y curl && \
    if [ "$BUILDARCH" = "arm64" ]; then \
      curl -sLO https://github.com/tailwindlabs/tailwindcss/releases/download/${TAILWIND_VERSION}/tailwindcss-linux-arm64; \
      mv tailwindcss-linux-arm64 tailwindcss; \
    else \
      curl -sLO https://github.com/tailwindlabs/tailwindcss/releases/download/${TAILWIND_VERSION}/tailwindcss-linux-x64; \
      mv tailwindcss-linux-x64 tailwindcss; \
    fi && \
    chmod +x tailwindcss

COPY . .

# Generate Swagger API docs
RUN /go/bin/swag init -g main.go --output api-docs --dir ./cmd/ramble,./internal/handlers,./internal/models --parseDependency --parseInternal

# Build CSS
RUN ./tailwindcss -i ./public/css/input.css -o ./public/css/style.css --minify

RUN CGO_ENABLED=0 GOOS=linux GOARCH=$TARGETARCH go build \
    -ldflags "-s -w -X rmbl/internal/cli/cmd.Version=${VERSION} -X rmbl/internal/cli/cmd.Commit=${COMMIT} -X rmbl/internal/cli/cmd.BuildDate=${BUILD_DATE}" \
    -o ramble ./cmd/ramble

# Stage 2: Final Image
FROM alpine:3.24

RUN apk --no-cache add ca-certificates \
 && addgroup -S -g 10001 ramble \
 && adduser -S -D -H -u 10001 -G ramble -s /sbin/nologin ramble

WORKDIR /app

# Copied without changing ownership: the app only reads views, public and docs, so root ownership
# keeps a compromised process from rewriting templates or static assets.
COPY --from=builder /app/ramble .
COPY --from=builder /app/views ./views
COPY --from=builder /app/public ./public
COPY --from=builder /app/docs ./docs

# Numeric so runAsNonRoot-style checks can verify it without reading /etc/passwd.
USER 10001:10001

EXPOSE 3000

CMD ["./ramble", "server"]
