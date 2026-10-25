# Regenerate with: make demo
Output demo/demo.gif

Require tmux

Set Shell bash
Set FontFamily "JetBrainsMono Nerd Font"
Set FontSize 16
Set Width 1200
Set Height 640
Set Padding 16
Set Theme "Catppuccin Mocha"
Set TypingSpeed 90ms

Hide
Type "./demo/setup.sh && source /tmp/thop-demo/bashrc && cd && clear" Enter
Sleep 8s
Type "tmux -L thopdemo -f /tmp/thop-demo/tmux.conf new -s home" Enter
Sleep 1s
Show

# Ranked by frecency
Type "thop" Enter
Sleep 2.5s

# Fuzzy find and open a repo
Type "api"
Sleep 1s
Enter
Sleep 1.5s

# Sibling repo opens as a second window in the same session
Type "thop" Enter
Sleep 1s
Type "acme/web"
Sleep 1s
Enter
Sleep 2s

# Clone into a project
Type "thop" Enter
Sleep 1s
Ctrl+G
Sleep 800ms
Type "charmbracelet/glow"
Sleep 800ms
Enter
Sleep 1s
Type "oss"
Sleep 1s
Enter
Sleep 4s

# Empty query + enter hops back
Type "thop" Enter
Sleep 2s
Enter
Sleep 1.5s

# Open sessions
Type "thop" Enter
Sleep 1s
Shift+Tab
Sleep 2.5s
Escape
Sleep 1s

Hide
Type "tmux kill-server" Enter
