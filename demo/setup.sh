#!/usr/bin/env bash
# Builds a throwaway HOME with fake projects for demo.tape.
set -euo pipefail

demo=${THOP_DEMO:-/tmp/thop-demo}
repo=$(cd "$(dirname "$0")/.." && pwd)

tmux -L thopdemo kill-server 2>/dev/null || true
rm -rf "$demo"
mkdir -p "$demo/bin" "$demo/home/.config/thop" "$demo/home/.local/share/thop"

go build -o "$demo/bin/thop" "$repo/cmd/thop"

p=$demo/home/projects
for d in dotfiles homelab website notes; do mkdir -p "$p/$d"; done
for d in api web infra; do mkdir -p "$p/acme/$d/.git"; done
for d in thop bubbletea lipgloss; do mkdir -p "$p/oss/$d/.git"; done
mkdir -p "$p/website/.git" "$p/dotfiles/.git"
mkdir -p "$demo/home/.cache/thop/tmp/scratch"

# Local stand-in for github.com/charmbracelet/glow (via insteadOf), so clone works offline.
src=$demo/src/glow
mkdir -p "$src"
git -C "$src" init -q
echo "# glow" > "$src/README.md"
git -C "$src" add README.md
git -C "$src" -c user.name=demo -c user.email=demo@example.com commit -qm init
git clone -q --bare "$src" "$demo/remote/charmbracelet/glow.git"
git config -f "$demo/home/.gitconfig" url."file://$demo/remote/".insteadOf https://github.com/

# Local clones finish instantly; slow them down so the clone dialog is visible.
cat > "$demo/bin/git" <<SH
#!/bin/sh
[ "\$1" = clone ] && sleep 2
exec $(command -v git) "\$@"
SH
chmod +x "$demo/bin/git"

cat > "$demo/home/.config/thop/config.yaml" <<YAML
paths:
  - ~/projects
popup:
  width: "70%"
  height: "70%"
YAML

now=$(date +%s)
{
	printf '%s\t%d\t%d\n' "$p/oss/thop" 30 "$now"
	printf '%s\t%d\t%d\n' "$p/dotfiles" 12 $((now - 7200))
	printf '%s\t%d\t%d\n' "$p/acme/api" 8 $((now - 90000))
	printf '%s\t%d\t%d\n' "$p/homelab" 4 $((now - 400000))
} > "$demo/home/.local/share/thop/history"

cat > "$demo/bashrc" <<RC
export HOME=$demo/home
export XDG_CONFIG_HOME=\$HOME/.config XDG_CACHE_HOME=\$HOME/.cache XDG_DATA_HOME=\$HOME/.local/share
export PATH=$demo/bin:\$PATH
export PS1='\[\e[33m\]\W\[\e[0m\] \$ '
RC

cat > "$demo/tmux.conf" <<CONF
set -g default-command "bash --rcfile $demo/bashrc"
set -g default-terminal "tmux-256color"
set -g status-style "bg=default,fg=white"
set -g status-left "#[fg=yellow,bold] #S #[default]"
set -g status-left-length 30
set -g status-right ""
set -g popup-border-lines rounded
set -g popup-style "bg=default,fg=default"
set -g popup-border-style "bg=default,fg=#b4befe"
CONF
