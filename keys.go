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
