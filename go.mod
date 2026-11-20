module flow

go 1.27.3

tool github.com/egoist/mygo/cmd/mygo

require (
	github.com/egoist/mygo v0.8.4-0.20261108082913-9233663736ca
	github.com/lobehub/lobe-mygo v0.0.0-20261109101238-5b3ecde8b184
	github.com/modelcontextprotocol/go-sdk v1.8.0
	golang.org/x/net v0.59.0
)

require (
	github.com/ebitengine/purego v0.11.1 // indirect
	github.com/go-text/typesetting v0.3.5 // indirect
	github.com/google/jsonschema-go v0.4.3 // indirect
	github.com/segmentio/asm v1.1.3 // indirect
	github.com/segmentio/encoding v0.5.4 // indirect
	github.com/yosida95/uritemplate/v3 v3.0.2 // indirect
	golang.org/x/image v0.46.0 // indirect
	golang.org/x/oauth2 v0.35.0 // indirect
	golang.org/x/sync v0.24.0 // indirect
	golang.org/x/sys v0.48.0 // indirect
	golang.org/x/text v0.43.0 // indirect
	golang.org/x/time v0.15.0 // indirect
)

// The UI is built on ./third_party/mygo, a copy of mygo v0.8.3 with local
// patches (durable media paste, editor clipping, text blocks and sections; see
// third_party/mygo/FLOW_PATCHES.md). The version required above is only the
// one Lobe MyGo asks for; this replace decides what is compiled.
replace github.com/egoist/mygo => ./third_party/mygo

// Lobe MyGo is ./third_party/lobe-mygo, the pinned version with one patch: an
// application may give the components its own palette (see
// third_party/lobe-mygo/FLOW_PATCHES.md).
replace github.com/lobehub/lobe-mygo => ./third_party/lobe-mygo
