# Norman Config

## Session Limits
max_tasks_per_session: 15
warn_at_tasks: 12

## Project
name: Ramble
repo: /Users/stokvis/dev/open-wander/core/ramble
created: 2026-05-30

## Commands (from Makefile / project)
build: go build -o bin/ramble ./cmd/ramble
test: go test ./...
security: govulncheck ./...
gosec: gosec ./...

## Subagent Defaults
default_subagent: golang-pro

## Model Strategy
advisor_mode: always
default_model: sonnet
quick_model: haiku
progress_compress_after: 10
