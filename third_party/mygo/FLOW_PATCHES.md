# Flow local patches

Base: github.com/egoist/mygo v0.8.3, unchanged upstream license retained.

- `ui/richtext.EditorOptions.OnPaste func() bool`: called before rich-text paste decoding for normal keyboard, native Edit-menu and context-menu pastes. A true result consumes the paste; false delegates to the original implementation. Paste and Match Style is unchanged.
- `ui/richtext/clipboard.go`: invokes that callback and stops text edit coalescing when consumed. Flow uses this to import clipboard files/images asynchronously into SQLite.

- `ui/richtext/paint.go`: clip the editor's own paint callback to its viewport. Element.Clip clips children, which previously allowed tall image attachments to paint over the toolbar after scrolling.

- Rich-text structural additions: `LineBreak` (U+2028), `ParagraphStyle.Divider`, `DividerRune`, `Document.InsertDivider` and `Edit.InsertDivider`. Shift+Enter keeps the paragraph/list item; `___` plus Space inserts a selectable rule. JSON, HTML, Markdown, plain text and RTF line-break import preserve their respective semantics. Rendering, caret navigation, typing/IME commits, rich pastes, boundary deletion and undo use the same document model. Flow adds `/divider` to its shared editor.

- Collapsible sections: flat paragraph metadata (`SectionDepth`, `SectionTitle`, `Collapsed`), `Document.MakeCollapsible`, `Document.ToggleSection`, `Text.SectionRange`, and transactional `Edit.MakeCollapsible` / `Edit.RemoveSection`. `>>>` plus Space creates a section. Folded paragraphs have zero layout height and retain complete text for serialization, search, copy and undo. Arrow navigation skips hidden content; destructive keys at folded boundaries first reveal it. Read-only editor toggles remain local to that view. Flow renders exported `<details>/<summary>` through native disclosure widgets in Markdown's HTML hook.

The full pinned module is copied here so builds are reproducible without editing the shared Go module cache. These opt-in changes are confined to `ui/richtext`; shared UI/backends are unchanged. Keep the replace until upstream offers equivalent media and text-block behavior.

Validation (Go 1.27.3, CGO_ENABLED=0): Darwin, Linux and Windows arm64 build and vet; production-tag vet; js/wasm vet excluding the CLI. Flow's full tests, race checks and native macOS file-picker/PNG-paste/restart path also passed. Phase16 adds richtext tests for paragraph/caret identity, Unicode, format round trips, divider clipboard/undo/typing/IME boundaries, and Flow draft restart/submission coverage.

Release-size measurements of applications that do not import richtext are identical before and after this patch, in bytes:

| arm64 target | counter-native | hello |
| --- | ---: | ---: |
| Darwin | 9,731,234 | 9,680,578 |
| Linux | 9,175,303 | 9,109,767 |
| Windows | 9,496,576 | 9,329,664 |

Commands and results are retained in `../../artifacts/implementation-2026-11-08/phase11-framework-{checks,sizes}.json`. Cross-platform compile checks are not native runtime checks on those platforms.

The full upstream `go test ./...` did not pass in this environment: CLI tests generate an unquoted local module replacement for paths containing spaces, and `examples/permissionflow.TestPermissions` cannot find the expected English accessibility labels. A clean, unmodified v0.8.3 copy under a path containing spaces reproduces `TestNativeTemplate` and `TestPermissions`. Logs: `phase11-framework-tests.log` and `phase11-framework-upstream-baseline.log`. The `ui`, `ui/richtext` and `ui/markdown` suites passed; these unrelated failures are retained rather than reported as a full framework pass.

Phase16 reran the full framework suite and retained the same categories of failure in `phase16-framework-all-tests.log`; `TestUpdateEndToEnd` also fails on the malformed local module path. Final targeted richtext checks and platform checks are recorded in `phase16-framework-*.log`. Darwin example binaries are byte-identical to pre-edit builds; Linux/Windows sizes match the recorded phase11 baseline. Native Flow verification covered slash and typed rules, list line breaks, undo/redo, quit/restart, submit and Markdown retrieval through the actual stdio helper. Full native IME interaction remains part of Flow's outstanding acceptance work.

Phase17 tests nested section JSON/HTML/Markdown round trips, media references, code blank lines, Enter/Backspace boundaries, folded EOF, rich paste, full-content copy, undo, read-only disclosure, whole-block actions and SQLite draft recovery. The complete framework run retains the same CLI/path and permission-example failures in `phase17-framework-all-tests.log`. Native Flow created FLW-22 from a recovered folded draft; actual Cmd+C/Cmd+V retained its complete nested body, and disclosure clicks in the reading view left SQLite unchanged. Native Unicode was pasted; real IME is not claimed. External Markdown HTML blocks separated by blank lines and full editor accessibility/keyboard parity remain follow-up work.
