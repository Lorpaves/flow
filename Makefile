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
	@mkdir -p .mygo
	@if [ "$$(uname -s)" = "Darwin" ] && command -v lipo >/dev/null 2>&1; then \
		GOOS=darwin GOARCH=arm64 go build -trimpath -o .mygo/flow-mcp-arm64 ./cmd/flow-mcp && \
		GOOS=darwin GOARCH=amd64 go build -trimpath -o .mygo/flow-mcp-amd64 ./cmd/flow-mcp && \
		lipo -create -output .mygo/flow-mcp .mygo/flow-mcp-arm64 .mygo/flow-mcp-amd64 && \
		rm -f .mygo/flow-mcp-arm64 .mygo/flow-mcp-amd64; \
	else \
		go build -trimpath -o .mygo/flow-mcp ./cmd/flow-mcp; \
	fi
icon:
	go run ./cmd/icon
