# Flow local patches

Base: github.com/lobehub/lobe-mygo v0.0.0-20261109101238-5b3ecde8b184 (only `go.mod`, `go.sum`, `LICENSE`, `README.md` and `pkg/`; the docs site, catalog and examples are left out). Upstream license retained.

- `pkg/theme/tokens.go`: `Customize(light, dark func(Tokens) Tokens)` replaces the palette that `Light`, `Dark`, `TokensOf` and `Apply` hand out. Every component reads its colors through `TokensOf`, so without it an application could recolor only what it draws itself. Flow calls it once at start-up with its own palette (`appearance.go`).

Nothing else differs from the pinned module. Keep the replace until upstream offers a way to theme the components.

Validation (Go 1.27.3, CGO_ENABLED=0, built through Flow's module so Flow's mygo copy is used): `pkg/theme` and `pkg/field` tests pass. The `pkg/action` and `pkg/choice` test files do not compile against Flow's mygo (they call `ui.Tester.AccessAll`, which mygo v0.8.3 does not have); that is a test-only gap between the versions and unrelated to this patch. Flow's own tests check that the components and Flow read the same palette (`appearance_palette_test.go`).
