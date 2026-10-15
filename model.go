package main

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var killFunc = func(pid, sig int) error { return syscall.Kill(pid, syscall.Signal(sig)) }

type sortMode int

const (
	sortPort sortMode = iota
	sortPID
	sortName
)

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

type (
	scanMsg       []PortEntry
	tickMsg       time.Time
	toastMsg      string
	toastClearMsg struct{}
	killResultMsg struct {
		pid, sig int
		err      error
	}
)

func newModel() model {
	ti := textinput.New()
	ti.Prompt = "filter: "
	ti.Placeholder = "port or process"
	return model{
		table: table.New(
			table.WithColumns([]table.Column{
				{Title: "PROTO", Width: 6},
				{Title: "LOCAL", Width: 24},
				{Title: "PORT", Width: 7},
				{Title: "PID", Width: 8},
				{Title: "PROCESS", Width: 20},
				{Title: "STATE", Width: 12},
			}),
			table.WithFocused(true),
			table.WithHeight(10),
		),
		filter: ti,
		keys:   newKeymap(),
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(scanCmd, tea.Every(2*time.Second, func(t time.Time) tea.Msg { return tickMsg(t) }))
}

func scanCmd() tea.Msg {
	entries, err := Scan()
	if err != nil {
		return toastMsg("scan: " + err.Error())
	}
	return scanMsg(entries)
}

func toastCmd(s string) tea.Cmd {
	return func() tea.Msg { return toastMsg(s) }
}

func killCmd(pid, sig int) tea.Cmd {
	return func() tea.Msg { return killResultMsg{pid: pid, sig: sig, err: killFunc(pid, sig)} }
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		m.table.SetWidth(msg.Width)
		if h := msg.Height - 4; h > 0 {
			m.table.SetHeight(h)
		}
		return m, nil
	case scanMsg:
		m.entries = msg
		m.setRows()
		return m, nil
	case tickMsg:
		return m, scanCmd
	case toastMsg:
		m.toast = string(msg)
		return m, tea.Tick(3*time.Second, func(time.Time) tea.Msg { return toastClearMsg{} })
	case toastClearMsg:
		m.toast = ""
		return m, nil
	case killResultMsg:
		switch {
		case msg.err == nil:
			if msg.sig == int(syscall.SIGTERM) {
				m.lastTerm = msg.pid
			} else {
				m.lastTerm = 0
			}
			return m, nil
		case errors.Is(msg.err, syscall.ESRCH):
			m.lastTerm = 0
			return m, scanCmd
		default:
			return m, toastCmd(fmt.Sprintf("kill %d: %v", msg.pid, msg.err))
		}
	case tea.KeyMsg:
		// handled below
	default:
		return m, nil
	}

	if k, ok := msg.(tea.KeyMsg); ok && k.String() == "ctrl+c" {
		return m, tea.Quit
	}

	if m.confirming {
		k := msg.(tea.KeyMsg)
		switch k.String() {
		case "y":
			m.confirming = false
			return m, killCmd(m.confirmPID, int(syscall.SIGTERM))
		case "n", "q", "esc":
			m.confirming = false
		}
		return m, nil
	}

	if m.filtering {
		k := msg.(tea.KeyMsg)
		switch k.String() {
		case "esc":
			m.filtering = false
			m.filterText = ""
			m.filter.SetValue("")
			m.setRows()
			return m, nil
		case "enter":
			m.filtering = false
			m.filterText = m.filter.Value()
			m.setRows()
			return m, nil
		}
		var cmd tea.Cmd
		m.filter, cmd = m.filter.Update(msg)
		return m, cmd
	}

	k := msg.(tea.KeyMsg)
	switch {
	case key.Matches(k, m.keys.Quit):
		return m, tea.Quit
	case key.Matches(k, m.keys.Kill):
		if e, ok := m.selected(); ok && e.PID > 0 {
			m.confirming = true
			m.confirmPID = e.PID
		}
		return m, nil
	case key.Matches(k, m.keys.ForceKill):
		if m.lastTerm > 0 {
			return m, killCmd(m.lastTerm, int(syscall.SIGKILL))
		}
		return m, nil
	case key.Matches(k, m.keys.Refresh):
		return m, scanCmd
	case key.Matches(k, m.keys.Sort):
		m.sort = (m.sort + 1) % 3
		m.setRows()
		return m, nil
	case key.Matches(k, m.keys.Filter):
		m.filtering = true
		m.filter.SetValue(m.filterText)
		return m, m.filter.Focus()
	}

	var cmd tea.Cmd
	m.table, cmd = m.table.Update(msg)
	return m, cmd
}

// selected returns the PortEntry under the table cursor.
func (m model) selected() (PortEntry, bool) {
	vis := m.visible()
	if len(vis) == 0 {
		return PortEntry{}, false
	}
	i := m.table.Cursor()
	if i < 0 || i >= len(vis) {
		return PortEntry{}, false
	}
	return vis[i], true
}

// visible returns entries filtered and sorted — same order setRows renders.
func (m model) visible() []PortEntry {
	var out []PortEntry
	for _, e := range m.entries {
		if m.filterText != "" &&
			!strings.Contains(strconv.Itoa(e.Port), m.filterText) &&
			!strings.Contains(strings.ToLower(e.Process), strings.ToLower(m.filterText)) {
			continue
		}
		out = append(out, e)
	}
	sort.SliceStable(out, func(i, j int) bool {
		switch m.sort {
		case sortPID:
			return out[i].PID < out[j].PID
		case sortName:
			return out[i].Process < out[j].Process
		default:
			return out[i].Port < out[j].Port
		}
	})
	return out
}

func (m *model) setRows() {
	vis := m.visible()
	rows := make([]table.Row, len(vis))
	for i, e := range vis {
		pid := "-"
		if e.PID > 0 {
			pid = strconv.Itoa(e.PID)
		}
		rows[i] = table.Row{e.Proto, e.Local, strconv.Itoa(e.Port), pid, e.Process, e.State}
	}
	m.table.SetRows(rows)
	if m.table.Cursor() < 0 && len(rows) > 0 {
		m.table.SetCursor(0)
	}
}

// forceKillHint reports whether the last SIGTERMd pid is still alive in the list.
func (m model) forceKillHint() bool {
	if m.lastTerm <= 0 {
		return false
	}
	for _, e := range m.entries {
		if e.PID == m.lastTerm {
			return true
		}
	}
	return false
}

var modalStyle = lipgloss.NewStyle().
	Border(lipgloss.RoundedBorder()).
	Padding(1, 3)

func (m model) View() string {
	if m.confirming {
		modal := modalStyle.Render(fmt.Sprintf("SIGTERM pid %d?  (y/n)", m.confirmPID))
		return lipgloss.Place(m.w, m.h, lipgloss.Center, lipgloss.Center, modal)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "port-checker — %d sockets\n", len(m.entries))
	if m.filtering {
		b.WriteString(m.filter.View())
	} else if m.filterText != "" {
		fmt.Fprintf(&b, "filter: %s", m.filterText)
	}
	b.WriteByte('\n')
	b.WriteString(m.table.View())
	b.WriteByte('\n')
	switch {
	case m.toast != "":
		b.WriteString(m.toast)
	case m.forceKillHint():
		fmt.Fprintf(&b, "pid %d still alive — press K for SIGKILL", m.lastTerm)
	}
	b.WriteByte('\n')
	b.WriteString("↑/k ↓/j move · x/enter kill · K force-kill · / filter · s sort · r refresh · q quit")
	return b.String()
}
