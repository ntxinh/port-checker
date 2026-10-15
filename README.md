# port-checker
Go + Bubble Tea TUI listing open ports and killing their owners.

## Run
```sh
mise install && make run
```

## Keybindings
| Key | Action |
|---|---|
| `↑`/`k`, `↓`/`j` | navigate |
| `enter` / `x` | SIGTERM selected port's owner (confirm) |
| `K` | SIGKILL last-SIGTERM'd PID if still alive |
| `/` | filter by port or process name (esc clears) |
| `s` | cycle sort: port → pid → process |
| `r` | refresh now |
| `q` / `ctrl+c` | quit |

See [docs/keybindings.md](docs/keybindings.md).

## License
MIT
