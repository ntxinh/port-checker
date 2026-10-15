# port-checker Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** TUI listing ports in use on the local machine, with confirm-then-SIGTERM/SIGKILL process kill.

**Architecture:** Flat `package main`. `scanner.go` reads `/proc/net/*` + `/proc/*/fd` → `[]PortEntry`; `model.go` is a Bubble Tea app (`bubbles/table` + `textinput`); scans run on a 2s `tea.Every` ticker in a goroutine. Confirm modal replaces content via `lipgloss.Place` (no `PlaceOverlay` in lipgloss v1.1).

**Tech Stack:** Go 1.27, `bubbletea v1.3.10`, `bubbles v1.0.0`, `lipgloss v1.1.0`. `tea.KeyMsg`/`tea.Key` API (v1, not v2 `KeyPressMsg`).

**Spec:** `docs/superpowers/specs/2026-10-15-port-checker-design.md` (== `DESIGN.md`)

## Global Constraints

- Linux-only; no `ss`/`lsof` subprocesses — parse `/proc` directly.
- Only deps: bubbletea, bubbles, lipgloss (+ their transitives). No gopsutil.
- Scans must never block `Update` — always via `tea.Cmd` goroutine.
- Unresolved PIDs (EPERM): `PID=0`, `Process="?"`, row stays listed.
- Errors → toast line, never crash, never silent drop except `ESRCH`.

---

### Task 1: Module + repo skeleton

**Files:**
- Create: `go.mod`, `Makefile`, `mise.toml`, `.editorconfig`, `AGENTS.md`, `README.md`, `docs/keybindings.md`, `main.go`

**Interfaces:**
- Produces: `func main()` calling `run()`; nothing else imports anything yet.

- [ ] **Step 1: Init module + deps**

```bash
cd /home/exodia/GitRepos/MyGits/port-checker
go mod init github.com/exodia/port-checker
go get github.com/charmbracelet/bubbletea@v1.3.10 github.com/charmbracelet/bubbles@v1.0.0 github.com/charmbracelet/lipgloss@v1.1.0
```

- [ ] **Step 2: Write skeleton files**

`main.go` (compiles; real wiring in Task 4):

```go
package main

import (
	"fmt"
	"os"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "port-checker:", err)
		os.Exit(1)
	}
}

func run() error { return nil }
```

`mise.toml`:

```toml
[tools]
go = "1.27.1"
"golangci-lint" = "latest"
```

`.editorconfig`:

```ini
root = true
[*]
indent_style = tab
indent_size = 4
end_of_line = lf
charset = utf-8
trim_trailing_whitespace = true
insert_final_newline = true
[*.md]
indent_style = space
indent_size = 2
[*.{yml,yaml,toml,json}]
indent_style = space
indent_size = 2
```

`Makefile`:

```make
BINARY := port-checker
.PHONY: build test lint run install clean
build:
	go build -o $(BINARY) .
test:
	go test ./...
lint:
	golangci-lint run ./...
run: build
	./$(BINARY)
install:
	go install .
clean:
	rm -f $(BINARY)
```

`AGENTS.md`:

```markdown
# port-checker
Go + Bubble Tea TUI listing open ports and killing their owners.

## Commands
- `make build` / `make test` / `make lint` / `make run`

## Conventions
- TDD: failing test before production code; `go test ./...` must stay green.
- Flat package `main`; new file only for a new responsibility.
- No subprocess calls (ss/lsof/fuser) — `/proc` parsing only.
- Deps: charmbracelet bubbletea/bubbles/lipgloss only. Ask before adding any dependency.
```

`README.md` (concise): project name, one-line purpose, `mise install && make run`, keybinding table (see docs/keybindings.md), license note MIT.

`docs/keybindings.md`:

```markdown
# Keybindings
| Key | Action |
|---|---|
| `↑`/`k`, `↓`/`j` | navigate |
| `enter` / `x` | SIGTERM selected port's owner (confirm) |
| `K` | SIGKILL last-SIGTERM'd PID if still alive |
| `/` | filter by port or process name (esc clears) |
| `s` | cycle sort: port → pid → process |
| `r` | refresh now |
| `q` / `ctrl+c` | quit |
```

- [ ] **Step 3: Verify + commit**

```bash
go build ./... && make test
git add -A && git commit -m "chore: module skeleton + repo files"
```

Expected: build clean, `no test files` OK.

---

### Task 2: `/proc/net/*` socket parser

**Files:**
- Create: `scanner.go` (parser half), `scanner_test.go`

**Interfaces:**
- Produces (used by Task 3+4):
  ```go
  type PortEntry struct {
      Proto   string // "tcp"|"tcp6"|"udp"|"udp6"
      Local   string // "127.0.0.1:8080"
      Port    int
      PID     int    // 0 = unresolved
      Process string // comm or "?"
      Cmdline string
      State   string // LISTEN, ESTABLISHED, ...
  }
  // parseProcNet parses /proc/net/{tcp,tcp6,udp,udp6} content.
  // Returns map[inode]*PortEntry.
  func parseProcNet(content string, proto string) (map[uint64]*PortEntry, error)
  ```

/proc/net format: header line, then fields; `local_address` = field 1 `HEXADDR:HEXPORT`, `st` = field 3 (hex state), `inode` = field 9. IPv4 addr hex is little-endian 4 bytes; IPv6 is 4 little-endian 32-bit words (byte-reverse each word). State map: `0A`→LISTEN, `01`→ESTABLISHED, `07`→CLOSE (UDP shows `07`), else `hex→"0x"+raw`.

- [ ] **Step 1: Write failing test — IPv4 + parse**

`scanner_test.go`:

```go
package main

import (
	"testing"
)

const tcpFixture = `  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode
   0: 0100007F:1F90 00000000:0000 0A 00000000:00000000 00:00000000 00000000  1000        0 12345 1 0000000000000000 100 0 0 10 0
   1: 00000000:0050 00000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 67890 1 0000000000000000 100 0 0 10 0`

func TestParseProcNetIPv4(t *testing.T) {
	m, err := parseProcNet(tcpFixture, "tcp")
	if err != nil {
		t.Fatal(err)
	}
	e := m[12345]
	if e == nil || e.Port != 8080 || e.Local != "127.0.0.1:8080" || e.State != "LISTEN" || e.Proto != "tcp" {
		t.Fatalf("bad entry: %+v", e)
	}
	if e2 := m[67890]; e2 == nil || e2.Port != 80 || e2.Local != "0.0.0.0:80" {
		t.Fatalf("bad entry2: %+v", e2)
	}
}
```

- [ ] **Step 2: Run — verify fail** (`undefined: parseProcNet`)

- [ ] **Step 3: Implement `parseProcNet` + `PortEntry` + hex decode**

`scanner.go`:

```go
package main

import (
	"encoding/hex"
	"fmt"
	"net"
	"strconv"
	"strings"
)

type PortEntry struct {
	Proto   string
	Local   string
	Port    int
	PID     int
	Process string
	Cmdline string
	State   string
	inode   uint64
}

var procStates = map[string]string{
	"01": "ESTABLISHED", "02": "SYN_SENT", "03": "SYN_RECV",
	"04": "FIN_WAIT1", "05": "FIN_WAIT2", "06": "TIME_WAIT",
	"07": "CLOSE", "08": "CLOSE_WAIT", "09": "LAST_ACK",
	"0A": "LISTEN", "0B": "CLOSING",
}

func parseProcNet(content, proto string) (map[uint64]*PortEntry, error) {
	out := map[uint64]*PortEntry{}
	for i, line := range strings.Split(content, "\n") {
		if i == 0 || strings.TrimSpace(line) == "" {
			continue
		}
		f := strings.Fields(line)
		if len(f) < 10 {
			continue
		}
		addr, port, err := decodeAddr(f[1], strings.HasSuffix(proto, "6"))
		if err != nil {
			continue // malformed line
		}
		inode, _ := strconv.ParseUint(f[9], 10, 64)
		if inode == 0 {
			continue
		}
		st := procStates[strings.ToUpper(f[3])]
		if st == "" {
			st = "0x" + f[3]
		}
		out[inode] = &PortEntry{
			Proto: proto,
			Local: addr + ":" + strconv.Itoa(port),
			Port:  port,
			State: st,
			inode: inode,
		}
	}
	return out, nil
}

func decodeAddr(field string, v6 bool) (string, int, error) {
	parts := strings.SplitN(field, ":", 2)
	if len(parts) != 2 {
		return "", 0, fmt.Errorf("bad addr %q", field)
	}
	raw, err := hex.DecodeString(parts[0])
	if err != nil {
		return "", 0, err
	}
	port64, err := strconv.ParseUint(parts[1], 16, 32)
	if err != nil {
		return "", 0, err
	}
	var ip net.IP
	if v6 {
		if len(raw) != 16 {
			return "", 0, fmt.Errorf("bad v6 len %d", len(raw))
		}
		ip = make(net.IP, 16)
		for w := 0; w < 4; w++ { // reverse each 32-bit word
			for b := 0; b < 4; b++ {
				ip[w*4+b] = raw[w*4+3-b]
			}
		}
	} else {
		if len(raw) != 4 {
			return "", 0, fmt.Errorf("bad v4 len %d", len(raw))
		}
		ip = net.IP{raw[3], raw[2], raw[1], raw[0]}
	}
	return ip.String(), int(port64), nil
}
```

- [ ] **Step 4: Run — verify pass** `go test -run TestParseProcNetIPv4 ./...`

- [ ] **Step 5: Failing tests — IPv6, UDP, malformed**

```go
func TestParseProcNetIPv6(t *testing.T) {
	fix := `  sl  local_address                         remote_address                        st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode
   0: 00000000000000000000000001000000:1F90 00000000000000000000000000000000:0000 0A 00000000:00000000 00:00000000 00000000  1000        0 999 1 0000000000000000 100 0 0 10 0`
	m, err := parseProcNet(fix, "tcp6")
	if err != nil {
		t.Fatal(err)
	}
	e := m[999]
	if e == nil || e.Port != 8080 || e.Local != "[::1]:8080" {
		t.Fatalf("got %+v", e)
	}
}
```

Wait — `Local` for v6 should be `::1:8080` or `[::1]:8080`? Decide now: `Local` stores `ip:port` joined plainly; for v6 wrap in brackets. Amend `parseProcNet`: `if strings.HasSuffix(proto,"6") { local = "["+addr+"]:"+port } else { addr+":"+port }`. Then expected `"[::1]:8080"`.

```go
func TestParseProcNetSkipsBadLines(t *testing.T) {
	m, err := parseProcNet("header\nbogus\n"+tcpFixture, "tcp")
	if err != nil || len(m) != 2 {
		t.Fatalf("got %d %v", len(m), err)
	}
}
```

- [ ] **Step 6: Run — verify fail on IPv6 (brackets), pass skip test; fix; all green**

- [ ] **Step 7: Commit** `git commit -m "feat: /proc/net socket parser"`

---

### Task 3: PID resolution + `Scan()`

**Files:**
- Modify: `scanner.go`, `scanner_test.go`

**Interfaces:**
- Produces:
  ```go
  // procRoot is a var so tests point at a fixture dir.
  var procRoot = "/proc"
  func Scan() ([]PortEntry, error) // resolved, all inodes, PID-filled where readable
  func resolvePIDs(inodes map[uint64]*PortEntry) // mutates: fills PID/Process/Cmdline
  ```
- Consumes: `parseProcNet` from Task 2.

- [ ] **Step 1: Failing test — fixture /proc**

```go
func TestScanResolvesPID(t *testing.T) {
	// fixture proc root built in test:
	//   <tmp>/net/tcp        (copy of tcpFixture, inode 12345)
	//   <tmp>/net/{tcp6,udp,udp6} (empty, header only)
	//   <tmp>/4321/fd/3 -> socket:[12345]  (symlink)
	//   <tmp>/4321/comm = "fakeproc\n"
	//   <tmp>/4321/cmdline = "fakeproc\x00--serve\x00"
	root := t.TempDir()
	writeFixtureProc(t, root) // helper below
	old := procRoot
	procRoot = root
	defer func() { procRoot = old }()

	got, err := Scan()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 entries, got %d", len(got))
	}
	var e *PortEntry
	for i := range got {
		if got[i].Port == 8080 {
			e = &got[i]
		}
	}
	if e == nil || e.PID != 4321 || e.Process != "fakeproc" || e.Cmdline != "fakeproc --serve" {
		t.Fatalf("got %+v", e)
	}
	// inode 67890 has no fd -> PID 0, Process "?"
	for i := range got {
		if got[i].Port == 80 && (got[i].PID != 0 || got[i].Process != "?") {
			t.Fatalf("unresolved row wrong: %+v", got[i])
		}
	}
}

func writeFixtureProc(t *testing.T, root string) {
	t.Helper()
	netDir := root + "/net"
	mkAll(t, netDir)
	writeFile(t, netDir+"/tcp", tcpFixture)
	for _, p := range []string{"tcp6", "udp", "udp6"} {
		writeFile(t, netDir+"/"+p, "  sl  local_address rem_address   st\n")
	}
	fd := root + "/4321/fd"
	mkAll(t, fd)
	if err := os.Symlink("socket:[12345]", fd+"/3"); err != nil {
		t.Fatal(err)
	}
	writeFile(t, root+"/4321/comm", "fakeproc\n")
	writeFile(t, root+"/4321/cmdline", "fakeproc\x00--serve\x00")
}

func mkAll(t *testing.T, p string)   { t.Helper(); if err := os.MkdirAll(p, 0o755); err != nil { t.Fatal(err) } }
func writeFile(t *testing.T, p, s string) {
	t.Helper()
	if err := os.WriteFile(p, []byte(s), 0o644); err != nil { t.Fatal(err) }
}
```

- [ ] **Step 2: Run — verify fail** (`undefined: Scan`)

- [ ] **Step 3: Implement `Scan` + `resolvePIDs`**

Append to `scanner.go`:

```go
import (
	"os"
	"path/filepath"
	"regexp"
)

var procRoot = "/proc"
var pidRe = regexp.MustCompile(`^[0-9]+$`)
var socketRe = regexp.MustCompile(`^socket:\[(\d+)\]$`)

func Scan() ([]PortEntry, error) {
	merged := map[uint64]*PortEntry{}
	for _, proto := range []string{"tcp", "tcp6", "udp", "udp6"} {
		b, err := os.ReadFile(filepath.Join(procRoot, "net", proto))
		if err != nil {
			continue // e.g. no ipv6 — fine
		}
		m, err := parseProcNet(string(b), proto)
		if err != nil {
			return nil, err
		}
		for k, v := range m {
			merged[k] = v
		}
	}
	resolvePIDs(merged)
	out := make([]PortEntry, 0, len(merged))
	for _, e := range merged {
		out = append(out, *e)
	}
	return out, nil
}

func resolvePIDs(inodes map[uint64]*PortEntry) {
	inodeToPID := map[uint64]int{}
	entries, _ := os.ReadDir(procRoot)
	for _, d := range entries {
		if !d.IsDir() || !pidRe.MatchString(d.Name()) {
			continue
		}
		pid, _ := strconv.Atoi(d.Name())
		fds, err := os.ReadDir(filepath.Join(procRoot, d.Name(), "fd"))
		if err != nil {
			continue // EPERM / exited
		}
		for _, fd := range fds {
			link, err := os.Readlink(filepath.Join(procRoot, d.Name(), "fd", fd.Name()))
			if err != nil {
				continue
			}
			if m := socketRe.FindStringSubmatch(link); m != nil {
				ino, _ := strconv.ParseUint(m[1], 10, 64)
				if _, seen := inodeToPID[ino]; !seen {
					inodeToPID[ino] = pid
				}
			}
		}
	}
	for ino, e := range inodes {
		pid := inodeToPID[ino]
		e.PID = pid
		e.Process = "?"
		if pid == 0 {
			continue
		}
		if b, err := os.ReadFile(filepath.Join(procRoot, strconv.Itoa(pid), "comm")); err == nil {
			e.Process = strings.TrimSpace(string(b))
		}
		if b, err := os.ReadFile(filepath.Join(procRoot, strconv.Itoa(pid), "cmdline")); err == nil {
			cl := strings.TrimRight(string(b), "\x00")
			e.Cmdline = strings.Join(strings.Split(cl, "\x00"), " ")
			if len(e.Cmdline) > 80 {
				e.Cmdline = e.Cmdline[:77] + "..."
			}
		}
	}
}
```

- [ ] **Step 4: Run — verify pass** `go test ./...`

- [ ] **Step 5: Live check** — `go run` a tiny main-less test? Do it via a throwaway test:

```go
func TestScanLive(t *testing.T) {
	// manual: go test -run TestScanLive -v
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil { t.Skip() }
	defer l.Close()
	got, err := Scan()
	if err != nil { t.Fatal(err) }
	port := l.Addr().(*net.TCPAddr).Port
	for _, e := range got {
		if e.Port == port && e.PID == os.Getpid() {
			return
		}
	}
	t.Fatalf("own listener :%d not found in %d entries", port, len(got))
}
```

Keep it — it's a real hermetic check (binds its own port). `go test ./...`.

- [ ] **Step 6: Commit** `git commit -m "feat: pid resolution + Scan"`

---

### Task 4: Bubble Tea UI + wiring

**Files:**
- Create: `model.go`, `keys.go`, `model_test.go`
- Modify: `main.go`

**Interfaces:**
- Produces: `newModel() model`, `model.Update`, `model.View`. `var killFunc = syscall.Kill` (injectable for tests).
- Consumes: `Scan()`, `PortEntry`.

Key logic:
- `Init`: `tea.Batch(scanCmd, tea.Every(2s, tickMsg))`.
- `scanCmd`: goroutine `Scan()` → `scanMsg`.
- `scanMsg` → set rows, and if `lastTerm > 0` && PID still in list → footer hint.
- Keys: `q/ctrl+c` quit; `x`/`enter` on row w/ PID>0 → `confirming` entry; `y` → `killCmd(pid, SIGTERM)`, store `lastTerm=pid`; `n/esc` cancel. `K` && `lastTerm` → `SIGKILL`. `r` → scanCmd. `s` → cycle sort re-render rows. `/` → filter mode (textinput); esc exits+clears; enter applies.
- Toast: `toastMsg` sets `m.toast`, `tea.Tick(3s)` clears via `toastClearMsg`.
- `killResultMsg{pid, sig, err}` → toast or set lastTerm; ESRCH → silent + scanCmd.
- Filter: substring on port int str or Process (case-insensitive).
- View: title line `port-checker — N sockets`, filter line when active, table (height `m.h-4`), toast/hint line, keybar. Confirming → `lipgloss.Place(m.w, m.h, Center, Center, modal)`.

- [ ] **Step 1: Failing model test**

```go
func TestKillFlow(t *testing.T) {
	m := newModel()
	m.w, m.h = 100, 30
	m.entries = []PortEntry{{Proto: "tcp", Port: 8080, Local: "127.0.0.1:8080", PID: 4321, Process: "fake", State: "LISTEN"}}
	m.setRows()
	var killed []int
	old := killFunc
	killFunc = func(pid, sig int) error { killed = append(killed, sig); return nil }
	defer func() { killFunc = old }()

	um, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	m = um.(model)
	if !m.confirming {
		t.Fatal("expected confirm modal")
	}
	um, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	m = um.(model)
	if cmd == nil {
		t.Fatal("expected kill cmd")
	}
	msg := cmd() // run the Cmd
	um, _ = m.Update(msg)
	m = um.(model)
	if len(killed) != 1 || killed[0] != int(syscall.SIGTERM) {
		t.Fatalf("killed=%v", killed)
	}
	if m.lastTerm != 4321 {
		t.Fatalf("lastTerm=%d", m.lastTerm)
	}
}
```

- [ ] **Step 2: Run — verify fail**

- [ ] **Step 3: Implement `keys.go`, `model.go`, wire `main.go`**

`keys.go`:

```go
package main

import "github.com/charmbracelet/bubbles/key"

type keymap struct {
	Up, Down, Kill, ForceKill, Filter, Sort, Refresh, Quit key.Binding
}

func newKeymap() keymap {
	return keymap{
		Up:        key.NewBinding(key.WithKeys("up", "k")),
		Down:      key.NewBinding(key.WithKeys("down", "j")),
		Kill:      key.NewBinding(key.WithKeys("enter", "x")),
		ForceKill: key.NewBinding(key.WithKeys("K")),
		Filter:    key.NewBinding(key.WithKeys("/")),
		Sort:      key.NewBinding(key.WithKeys("s")),
		Refresh:   key.NewBinding(key.WithKeys("r")),
		Quit:      key.NewBinding(key.WithKeys("q", "ctrl+c")),
	}
}
```

`model.go` (structure — implement fully):

```go
type sortMode int
const (sortPort sortMode = iota; sortPID; sortName)

type model struct {
	table      table.Model
	filter     textinput.Model
	keys       keymap
	entries    []PortEntry // unfiltered
	w, h       int
	sort       sortMode
	filtering  bool
	filterText string
	confirming bool
	confirmPID int
	lastTerm   int // pid we SIGTERMd; >0 enables K hint + force kill
	toast      string
}
```

Msgs: `scanMsg []PortEntry`, `tickMsg time.Time`, `killResultMsg{pid, sig int, err error}`, `toastClearMsg`.

`main.go run()`:

```go
func run() error {
	if _, err := os.Stat("/proc/net/tcp"); err != nil {
		return fmt.Errorf("/proc not available (Linux only)")
	}
	p := tea.NewProgram(newModel(), tea.WithAltScreen())
	_, err := p.Run()
	return err
}
```

- [ ] **Step 4: Run — verify pass** `go test ./...`

- [ ] **Step 5: Add sort/filter/toast tests** (each fail→impl already done in step 3 code → keep honest: if tests pass immediately, that's fine for behaviors implemented in same step; TDD loop was on kill-flow. For ponytail scope: add `TestFilter`, `TestSortCycle`, `TestKillResultESRCHSilent` compactly; implement alongside if they fail.)

- [ ] **Step 6: Build + manual smoke**

```bash
make build && ./port-checker
```

Verify: table renders own shell's ports, `x`→`y` on a test listener (`python3 -m http.server 9999 &`) SIGTERMs it, `q` quits clean.

- [ ] **Step 7: Commit** `git commit -m "feat: tui + kill flow"`

---

### Task 5: Polish + docs

**Files:**
- Modify: `README.md` (real keybinding table + usage gif placeholder line), `docs/keybindings.md` final check vs `keys.go`, `Makefile` (`release` target optional — skip, YAGNI).

- [ ] **Step 1: Diff `keys.go` vs `docs/keybindings.md`; sync**
- [ ] **Step 2: `golangci-lint run` via `make lint`; fix findings**
- [ ] **Step 3: `go vet ./...`, `gofmt -l .` clean**
- [ ] **Step 4: Final commit** `git commit -m "docs: readme + keybinding sync"`

---

## Self-review notes

- Spec coverage: parser ✓(T2), PID resolution ✓(T3), TUI/sort/filter/kill/toast/ticker ✓(T4), repo files ✓(T1), tests ✓(each), smoke ✓(T4.6).
- `PortEntry.inode` unexported — fine (same package).
- `killFunc` signature `func(pid, sig int) error` wraps `syscall.Kill`; test injects before Update.
- lipgloss v1.1: use `lipgloss.Place` not PlaceOverlay (verified absent).
