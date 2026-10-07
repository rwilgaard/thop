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
thop -s                # start in the Open filter (active sessions)
thop ~/projects/myapp  # open a path directly, no picker
thop clone <url>       # pick a destination and clone
thop clone owner/repo  # shorthand, expands via clone_shorthand
thop tmp               # create a new tmp project and open it
thop tmp myname        # create a named tmp project
thop --version         # print version
```

Inside tmux, `thop` opens as a popup. Outside tmux it runs inline.

With an empty query the session you came from is listed first and the one you're in last, so `thop` then `Enter` hops back.

### Keys

| Key | Action |
|-----|--------|
| Type | Filter |
| `↑` / `Ctrl-K` | Move up (wraps) |
| `↓` / `Ctrl-J` | Move down (wraps) |
| `PgUp` / `Ctrl-U` | Page up |
| `PgDn` / `Ctrl-D` | Page down |
| `Enter` | Open |
| `Tab` / `Shift-Tab` | Next / previous filter |
| `Ctrl-Q` | Close the highlighted session or window |
| `Ctrl-G` | Clone a git repo (`Esc` cancels a running clone) |
| `Ctrl-N` | New project |
| `Ctrl-T` | New tmp project |
| `Ctrl-X` | Delete tmp projects |
| `?` | Toggle full keymap |
| `Esc` / `Ctrl-C` | Quit |

The status bar shows the filters (`All • Projects • Repos • Tmp • Open`) with the active one highlighted, and the cursor position (`3/120`). It switches to mode-specific text during clone/tmp/clean flows.

`Ctrl-Q` asks before closing. On a project it kills the whole session, on a repo just its window.

### New projects

`Ctrl-N` creates a directory under one of your project roots and puts the cursor on it. With several roots you pick one first. Nothing is opened and no `git init` is run.

### Tmp projects

`Ctrl-T` creates a disposable scratch directory under `tmp_path` and opens it immediately as a tmux session. Projects appear in the picker with a `~` icon.

`Ctrl-X` opens a delete mode: type to filter the list, `Space` to select specific projects, `Enter` to confirm, `Esc` to cancel. With nothing selected, only the highlighted project is deleted. Open tmux sessions of deleted projects are killed.

## Configuration

First run creates `~/.config/thop/config.yaml` and asks for a project root, which it saves there. Add more roots in the file:

```yaml
paths:
  - ~/projects
  - ~/work

# tmp_path: ~/scratch  # defaults to ~/.cache/thop/tmp
# layout: top           # or "bottom": status bar top, search bar bottom, list reversed
# clone_shorthand: "https://github.com/{repo}.git"  # what "owner/repo" expands to when cloning

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
#   newtmp: ["ctrl+t"]
#   newproject: ["ctrl+n"]
#   cleantmp: ["ctrl+x"]
#   close: ["ctrl+q"]
#   pageup: ["pgup", "ctrl+u"]
#   pagedown: ["pgdown", "ctrl+d"]
#   nextfilter: ["tab"]
#   prevfilter: ["shift+tab"]
```

Filters can also be bound to direct keys. These have no defaults:

```yaml
# keymap:
#   all: ["alt+1"]
#   projects: ["alt+2"]
#   repos: ["alt+3"]
#   tmp: ["alt+4"]
#   open: ["alt+5"]
```

Binding a plain character (like `k`) makes it untypeable in the search field, so stick to modifier keys for anything you'd also want to type. A key you bind is taken away from the action that had it by default. Binding one key to two actions yourself is rejected at startup.

Icons default to Nerd Font glyphs. Override any of them (use plain ASCII if your font lacks the glyphs). Omit an entry to keep its default:

```yaml
# icons:
#   project: ""    # project dir icon
#   repo: ""       # git repo icon
#   tmp: "~"       # tmp project icon
#   prompt: ">"    # search/input prompt glyph
#   active: "*"    # open-session indicator
#   selected: "x"  # multi-select check mark
#   warning: "!"   # warning glyph
#   separator: "-" # horizontal rule rune
```

Terminals send `ctrl+i`, `ctrl+m` and `ctrl+[` as `tab`, `enter` and `esc`, so thop binds both spellings: `clone: ["ctrl+i"]` also triggers on tab (and collides with the default `nextfilter`, so rebind that too).

`alt+<key>`, `shift+<named key>` (like `shift+tab`) and function keys work everywhere. Combos like `ctrl+shift+x` need the enhanced keyboard protocol: a terminal that supports it (kitty, ghostty, WezTerm, recent iTerm2) and `set -s extended-keys on` in your tmux config. Without it the key degrades to plain `ctrl+x`. Modifiers must be spelled in the order `ctrl`, `alt`, `shift`, with a lowercase letter.

## How it works

**Sessions and windows** — Projects open as sessions. Git repos inside a project open as windows in that session, with the session root set to the project directory so new windows land there by default.

**Same name in two roots** — If `~/code/app` and `~/work/app` both exist, the picker shows each with its root and the sessions are named `app@code` and `app@work`. Unique names keep a plain session name.

**Scanning** — Each configured path is scanned two levels deep. Directories with `.git` are treated as repos. A configured path that doesn't exist is named in a warning row at the bottom of the list.

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
