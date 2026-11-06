## 1. Core implementation

- [x] 1.1 Create `loadWorkspaceContext(workspaceDir string) string` function in `internal/cmd/serve.go` that reads AGENTS.md, SOUL.md, USER.md, IDENTITY.md, TOOLS.md in order
- [x] 1.2 Add per-file truncation at 20,000 characters with warning log
- [x] 1.3 Add total context truncation at 60,000 characters with warning log
- [x] 1.4 Wrap loaded content in `## Project Context` section with `### <FILENAME>` headers per file
- [x] 1.5 Log loaded files and total character count at INFO level

## 2. Integration

- [x] 2.1 Integrate `loadWorkspaceContext()` into serve command's prompt assembly -- append after system-prompt.txt content and workspace path injection
- [x] 2.2 Ensure workspace context loads regardless of whether agent-config.yaml exists (works in both phase 1 and phase 2 modes)

## 3. Testing

- [x] 3.1 Add unit test: all five files present -- verify order and section headers
- [x] 3.2 Add unit test: partial files -- verify missing files skipped silently
- [x] 3.3 Add unit test: no files present -- verify empty string returned
- [x] 3.4 Add unit test: per-file truncation at 20K chars
- [x] 3.5 Add unit test: total truncation at 60K chars
- [x] 3.6 Add test fixtures in `testdata/` with sample OpenClaw workspace files

## 4. Documentation and deployment

- [x] 4.1 Add example OpenClaw workspace files to `testdata/` or `examples/`
- [x] 4.2 Update deployment manifest examples to show ConfigMap mounting of workspace files
- [x] 4.3 Run `make lint` and `make test` to verify no regressions
