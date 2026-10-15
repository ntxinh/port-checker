package main

import (
	"syscall"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

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

func TestFilter(t *testing.T) {
	m := newModel()
	m.w, m.h = 100, 30
	m.entries = []PortEntry{
		{Proto: "tcp", Port: 80, Local: "0.0.0.0:80", PID: 1, Process: "nginx", State: "LISTEN"},
		{Proto: "tcp", Port: 8080, Local: "127.0.0.1:8080", PID: 2, Process: "GoLand", State: "LISTEN"},
		{Proto: "udp", Port: 53, Local: "0.0.0.0:53", PID: 3, Process: "named", State: ""},
	}

	m.filterText = "gol" // case-insensitive process substring
	m.setRows()
	if got := len(m.table.Rows()); got != 1 {
		t.Fatalf("process filter rows=%d", got)
	}

	m.filterText = "80" // port substring matches 80 and 8080
	m.setRows()
	if got := len(m.table.Rows()); got != 2 {
		t.Fatalf("port filter rows=%d", got)
	}

	m.filterText = ""
	m.setRows()
	if got := len(m.table.Rows()); got != 3 {
		t.Fatalf("no filter rows=%d", got)
	}
}

func TestSortCycle(t *testing.T) {
	m := newModel()
	m.w, m.h = 100, 30
	m.entries = []PortEntry{
		{Proto: "tcp", Port: 9000, Local: ":9000", PID: 5, Process: "zzz"},
		{Proto: "tcp", Port: 22, Local: ":22", PID: 9, Process: "aaa"},
		{Proto: "tcp", Port: 443, Local: ":443", PID: 1, Process: "mmm"},
	}
	m.setRows()
	if got := m.table.Rows()[0][2]; got != "22" { // default: by port
		t.Fatalf("sortPort first=%q", got)
	}

	um, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	m = um.(model)
	if got := m.table.Rows()[0][3]; got != "1" { // by pid
		t.Fatalf("sortPID first pid=%q", got)
	}

	um, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	m = um.(model)
	if got := m.table.Rows()[0][4]; got != "aaa" { // by name
		t.Fatalf("sortName first=%q", got)
	}

	um, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	m = um.(model)
	if m.sort != sortPort || m.table.Rows()[0][2] != "22" {
		t.Fatalf("cycle failed: sort=%d first=%q", m.sort, m.table.Rows()[0][2])
	}
}

func TestKillResultESRCHSilent(t *testing.T) {
	m := newModel()
	m.w, m.h = 100, 30
	m.lastTerm = 4321
	um, cmd := m.Update(killResultMsg{pid: 4321, sig: int(syscall.SIGTERM), err: syscall.ESRCH})
	m = um.(model)
	if m.toast != "" {
		t.Fatalf("toast=%q, want silent", m.toast)
	}
	if m.lastTerm != 0 {
		t.Fatalf("lastTerm=%d", m.lastTerm)
	}
	if cmd == nil {
		t.Fatal("expected rescan cmd")
	}
}

func TestCursorRecoveryAfterEmptyFilter(t *testing.T) {
	m := newModel()
	m.w, m.h = 100, 30
	m.entries = []PortEntry{{Proto: "tcp", Port: 8080, Local: "127.0.0.1:8080", PID: 4321, Process: "fake", State: "LISTEN"}}
	m.setRows()

	// filter drains rows to 0 → cursor goes to -1
	m.filterText = "zzz"
	m.setRows()
	if len(m.table.Rows()) != 0 {
		t.Fatal("expected empty table")
	}

	// clearing the filter refills rows; cursor must recover
	m.filterText = ""
	m.setRows()
	if m.table.Cursor() != 0 {
		t.Fatalf("cursor=%d", m.table.Cursor())
	}

	// x on the recovered row opens the confirm modal
	um, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	m = um.(model)
	if !m.confirming {
		t.Fatal("x did not open confirm modal after cursor recovery")
	}
}

func TestCtrlCQuitsEverywhere(t *testing.T) {
	ctrlC := tea.KeyMsg{Type: tea.KeyCtrlC}

	// base state
	m := newModel()
	if _, cmd := m.Update(ctrlC); cmd == nil {
		t.Fatal("ctrl+c in base state: no cmd")
	}

	// confirming state
	m = newModel()
	m.confirming = true
	m.confirmPID = 4321
	if _, cmd := m.Update(ctrlC); cmd == nil {
		t.Fatal("ctrl+c in confirming state: no cmd")
	}

	// filtering state
	m = newModel()
	m.filtering = true
	if _, cmd := m.Update(ctrlC); cmd == nil {
		t.Fatal("ctrl+c in filtering state: no cmd")
	}

	// q cancels the confirm modal (does not quit)
	m = newModel()
	m.confirming = true
	m.confirmPID = 4321
	um, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	m = um.(model)
	if m.confirming {
		t.Fatal("q did not cancel confirm modal")
	}
	if cmd != nil {
		t.Fatal("q in confirm returned a cmd")
	}
}
