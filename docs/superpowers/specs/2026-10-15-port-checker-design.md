# port-checker — Design

TUI to list ports in use on the local machine and kill owning processes.
Go + Bubble Tea. Reference UX: lazygit (keyboard-driven panels, confirm popups, footer keybar).

## Architecture

Single module, flat package `main` — ~500 LOC.

| File | Responsibility |
|---|---|
| `main.go` | entry, `tea.NewProgram` wiring, flags |
| `scanner.go` | `/proc` parsing → `[]PortEntry` |
| `model.go` | Bubble Tea model: table, modals, ticks |
| `keys.go` | keymap (bubbles/key) |
| `scanner_test.go` | fixture-driven parser tests |

Deps: `bubbletea`, `bubbles` (table, key, textinput), `lipgloss`. Nothing else.

## Data model

```go
type PortEntry struct {
    Proto   string // "tcp" | "tcp6" | "udp" | "udp6"
    Local   string // "127.0.0.1:8080"
    Port    int
    PID     int    // 0 = unresolved (other user's process)
    Process string // comm
    Cmdline string // joined argv, truncated
    State   string // LISTEN, ESTABLISHED, ...
}
```

## Scanning (scanner.go)

1. Parse `/proc/net/{tcp,tcp6,udp,udp6}` → map `inode → {proto, local addr:port, state}`.
   - Local address field is hex `ADDR:PORT`; decode per proto (IPv4 = 4 bytes LE, IPv6 = 16 bytes).
2. Walk `/proc/[0-9]*/fd/` symlinks; `readlink` match `socket:[inode]` → PID.
3. For each matched PID read `/proc/PID/comm` + `/proc/PID/cmdline` (NUL-separated → join).
4. Unreadable `/proc` entries (EPERM, other users): socket stays in list with `PID=0`, `Process="?"`.

Include all sockets that have a local port bound (LISTEN, ESTABLISHED, UDP unconnected). No filtering by state — show what `ss -tulpn` would, minus `ss` dependency.

## UI (model.go)

- `bubbles/table` full-screen: columns PORT, PROTO, PID, PROCESS, LOCAL ADDR, CMDLINE.
- Footer keybar (lipgloss): `↑/k ↓/j` navigate · `enter`/`x` kill · `/` filter · `s` sort · `r` refresh · `q` quit.
- Auto-refresh: `tea.Tick` every 2s; scan on a goroutine, result arrives as `tea.Msg` (table never blocks).
- Sort: `s` cycles port → pid → process name.
- Filter: `/` opens `textinput` at top; matches substring on port or process name; `esc` clears.
- Kill: `enter`/`x` → centered modal `SIGTERM pid 1234 (nginx) on :8080? [y/N]` → `SIGTERM`. If PID still present after next 2s refresh, footer hint `still alive — press K for SIGKILL`. `K` → `SIGKILL`, no second confirm.
- Errors surface as a one-line toast (bottom, auto-clears in 3s): `EPERM killing 1234`, `process gone`, etc.

## Error handling

- `/proc` read races (process exits mid-scan): skip entry, never fatal.
- `SIGTERM`/`SIGKILL` errors → toast; `ESRCH` treated as already-dead (silent refresh).
- Non-Linux or missing `/proc`: print error to stderr, exit 1.

## Testing

- `scanner_test.go`: table-driven tests over fixture `/proc/net/*` text — hex decode, inode map, IPv6. PID resolution tested via `os.Readlink`-swappable function var.
- Model kill-flow: `teatest` driving `x`,`y` against a stubbed kill func.
- Manual smoke: `go run .`, open a listener, kill it.

## Repo files

`go.mod`, `Makefile` (build/test/lint/run/install), `mise.toml` (go, golangci-lint), `AGENTS.md`, `README.md`, `.editorconfig`, `DESIGN.md` (copy of this spec), `docs/` (keybindings + screenshots dir, minimal).
