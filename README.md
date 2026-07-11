# thop

> hop between tmux sessions

Fuzzy picker for tmux sessions. Scans your project directories, ranks results by frecency, and opens as a floating popup when you're already inside tmux.

Git repos nested inside a project open as windows in that project's session rather than their own top-level session.

## Requirements

- Go 1.26+
- tmux

## Installation

```sh
go install github.com/rwilgaard/thop/cmd/thop@latest
```

Or build from source:

```sh
make install
```

## Usage

```sh
thop                   # open picker
thop -s                # only show active sessions
thop ~/projects/myapp  # open a path directly, no picker
thop clone <url>       # pick a destination and clone
thop tmp               # create a new tmp project and open it
thop tmp myname        # create a named tmp project
thop --version         # print version
```

Inside tmux, `thop` opens as a popup. Outside tmux it runs inline.

### Keys

| Key | Action |
|-----|--------|
| Type | Filter |
| `↑` / `Ctrl-K` | Move up |
| `↓` / `Ctrl-J` | Move down |
| `Enter` | Open |
| `Ctrl-A` | Show all |
| `Ctrl-P` | Projects only |
| `Ctrl-R` | Repos only |
| `Ctrl-T` | Tmp only |
| `Ctrl-G` | Clone a git repo |
| `Ctrl-N` | New tmp project |
| `Ctrl-X` | Delete tmp projects |
| `?` | Toggle full keymap |
| `Esc` / `Ctrl-C` | Quit |

The status bar shows the active filter (`^A All · ^P Projects · ^R Repos · ^T Tmp`) and item count, and switches to mode-specific text during clone/tmp/clean flows.

### Tmp projects

`Ctrl-N` creates a disposable scratch directory under `tmp_path` and opens it immediately as a tmux session. Projects appear in the picker with a `~` icon.

`Ctrl-X` opens a delete mode: type to filter the list, `Space` to select specific projects, `Enter` to confirm, `Esc` to cancel. With nothing selected, all tmp projects are deleted after confirmation.

## Configuration

First run creates `~/.config/thop/config.yaml`. Add your project roots:

```yaml
paths:
  - ~/projects
  - ~/work

# tmp_path: ~/scratch  # defaults to ~/.cache/thop/tmp
# layout: top           # or "bottom": status bar top, search bar bottom, list reversed

# popup:                # size of the tmux popup thop opens itself in
#   width: "60%"         # any tmux -w value (percent or fixed cols)
#   height: "50%"        # any tmux -h value (percent or fixed rows)
```

Colors default to your terminal palette. Override with terminal color numbers (`0`–`255`) or hex codes:

```yaml
# colors:
#   selection_bg: "8"
#   selection_fg: "15"
#   active_color: "11"
#   prompt_color: "11"
#   match_color: ""       # defaults to prompt_color if unset
#   status_active_color: "11"
#   help_key_color: ""
#   help_desc_color: ""
```

Every keybinding can be remapped. Omit any entry to keep its default:

```yaml
# keymap:
#   up: ["up", "ctrl+k"]
#   down: ["down", "ctrl+j"]
#   enter: ["enter"]
#   quit: ["esc", "ctrl+c"]
#   help: ["?"]
#   clone: ["ctrl+g"]
#   newtmp: ["ctrl+n"]
#   cleantmp: ["ctrl+x"]
#   all: ["ctrl+a"]
#   projects: ["ctrl+p"]
#   repos: ["ctrl+r"]
#   tmp: ["ctrl+t"]
```

Binding a plain character (like `k`) makes it untypeable in the search field, so stick to modifier keys for anything you'd also want to type. Binding the same key to two actions is rejected at startup.

Terminals send `ctrl+i`, `ctrl+m` and `ctrl+[` as `tab`, `enter` and `esc`, so thop binds both spellings: `clone: ["ctrl+i"]` also triggers on tab.

`alt+<key>`, `shift+<named key>` (like `shift+tab`) and function keys work everywhere. Combos like `ctrl+shift+x` need the enhanced keyboard protocol: a terminal that supports it (kitty, ghostty, WezTerm, recent iTerm2) and `set -s extended-keys on` in your tmux config. Without it the key degrades to plain `ctrl+x`. Modifiers must be spelled in the order `ctrl`, `alt`, `shift`, with a lowercase letter.

## How it works

**Sessions and windows** — Projects open as sessions. Git repos inside a project open as windows in that session, with the session root set to the project directory so new windows land there by default.

**Scanning** — Each configured path is scanned two levels deep. Directories with `.git` are treated as repos.

**Frecency** — Selections are tracked and ranked with a 60/40 blend of fuzzy match score and frecency, so frequently visited paths surface quickly even with short queries.

**Startup scripts** — After creating a new session, thop sources `.thop` in the project directory, falling back to `~/.thop`. Handy for window layouts, env vars, and so on.

## File locations

Respects XDG dirs if set.

| Purpose | Default |
|---------|---------|
| Config | `~/.config/thop/config.yaml` |
| Candidate cache | `~/.cache/thop/candidates` |
| Frecency history | `~/.local/share/thop/history` |
| Tmp projects | `~/.cache/thop/tmp` |
