export GOTOOLCHAIN := go1.27.3
export CGO_ENABLED := 0

.PHONY: dev test check build release icon mcp
dev: mcp
	FLOW_DATA_DIR="$(CURDIR)/.local/dev" go tool mygo dev
test:
	go test ./...
check:
	go test ./...
	go tool mygo vet .
build:
	go tool mygo build -skip-notarize
release:
	go tool mygo build -skip-notarize -upload
mcp:
	go build -trimpath -o .mygo/flow-mcp ./cmd/flow-mcp
icon:
	go run ./cmd/icon
