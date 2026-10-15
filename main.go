package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "port-checker:", err)
		os.Exit(1)
	}
}

func run() error {
	if _, err := os.Stat("/proc/net/tcp"); err != nil {
		return fmt.Errorf("/proc not available (Linux only)")
	}
	p := tea.NewProgram(newModel(), tea.WithAltScreen())
	_, err := p.Run()
	return err
}
