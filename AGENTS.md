# Repository Guidelines

## Project Structure & Module Organization
- Root Go module: `go.mod` (Go 1.24.5). All source files live at repo root (e.g., `main.go`, `deepseek.go`, `config.go`).
- Binary output: `bin/mcp-deepseek` (created by build step).
- Scripts: `run_test.sh`, `run_lint.sh`, `run_format.sh` for common workflows.
- Config: `.env` for local environment variables; see `README.md` for supported keys.

## Build, Test, and Development Commands
- Build: `go build -o bin/mcp-deepseek` — compiles the MCP server.
- Run: `DEEPSEEK_API_KEY=... DEEPSEEK_MODEL=deepseek-chat ./bin/mcp-deepseek` — starts the server.
- Tests: `./run_test.sh` or `go test -v ./...` — runs all packages.
- Lint: `./run_lint.sh` (uses `golangci-lint`) — static analysis and quick fixes.
- Format: `./run_format.sh` or `gofmt -w .` — formats Go files.

## Coding Style & Naming Conventions
- Go formatting: use `gofmt`. CI reviewers may request reformatting before merge.
- Packages: lowercase, no underscores (e.g., `mcp`). Files: lowercase with optional underscores (e.g., `prompt_handlers.go`).
- Exports: PascalCase for exported types/functions; unexported identifiers use camelCase.
- Errors: return `error` as the last value; wrap with `%w` and include context.
- Context: accept `ctx context.Context` as the first param in long‑running or external calls.

## Testing Guidelines
- Place tests alongside code: `foo_test.go`. Prefer table‑driven tests.
- Run coverage locally: `go test -cover ./...`.
- Aim to cover configuration parsing, API boundaries, and prompt/tool behaviors.

## Commit & Pull Request Guidelines
- Commits: follow Conventional Commits seen in history (e.g., `feat(deepseek): ...`, `fix(security): ...`, `chore(deps): ...`).
- PRs: include a clear description, linked issues, reproduction steps, and screenshots/logs when relevant.
- Requirements: passing `go test`, clean `golangci-lint`, and formatted code.

## Security & Configuration Tips
- Never commit secrets. Use `.env` locally and environment variables in CI.
- Validate inputs and respect file limits and allowed MIME types (see `deepseek.go` and `config.go`).
- When adding new tools or prompts, prefer explicit whitelists and sensible timeouts.

## Architecture Notes
- Entry point: `main.go` wires configuration, logging, and MCP tools.
- Core behavior lives in `deepseek.go`, `prompts.go`, and `prompt_handlers.go` with supporting utilities in `config.go` and `logger.go`.

