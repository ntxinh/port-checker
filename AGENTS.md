# port-checker
Go + Bubble Tea TUI listing open ports and killing their owners.

## Commands
- `make build` / `make test` / `make lint` / `make run`

## Conventions
- TDD: failing test before production code; `go test ./...` must stay green.
- Flat package `main`; new file only for a new responsibility.
- No subprocess calls (ss/lsof/fuser) — `/proc` parsing only.
- Deps: charmbracelet bubbletea/bubbles/lipgloss only. Ask before adding any dependency.
