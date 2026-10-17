package config

// DefaultFile is printed by knot --print-default-config. Every value equals
// the built-in default.
const DefaultFile = `# knot configuration file.
# Location (first found wins): --config <file>, $KNOT_CONFIG,
# $XDG_CONFIG_HOME/knot/knot.toml, ~/.config/knot/knot.toml

# Theme name: a built-in theme (default, tig-classic, dark, light,
# solarized-dark, solarized-light, gruvbox, catppuccin-mocha), a user theme in
# <config dir>/themes/<name>.toml, or "auto" to choose by terminal background.
theme = "default"

[theme-auto]
dark  = "dark"   # theme used on a dark terminal background when theme = "auto"
light = "light"  # theme used on a light terminal background when theme = "auto"

[ui]
line-graphics     = "utf-8"       # utf-8 | ascii | default
split-direction   = "horizontal"  # horizontal | vertical | auto
split-ratio       = 0.66          # size of the child view in a split (0.1 - 0.9)
show-line-numbers = false
mouse             = true
tab-size          = 8
title-overflow    = 50            # highlight subjects longer than this ($ toggles)

[main]
columns          = ["date", "author", "commit-id", "graph", "refs", "subject"]
date-format      = "absolute"     # off | absolute | relative | relative-compact | custom
date-custom      = "%Y-%m-%d %H:%M"
author           = "full"         # off | full | abbreviated | email | email-user
commit-id-length = 7
graph            = true

[diff]
context           = 3
ignore-whitespace = false
highlight         = "line"        # off | line | word

[blame]
columns = ["commit-id", "author", "date", "line-number"]

[worktree]
on-switch               = "main"  # view opened after switching worktree
show-dirty              = true
dirty-check-concurrency = 4
open-command            = ["$SHELL"]  # argv, run with cwd = worktree path

[git]
binary   = "git"
log-args = []

# Key bindings per view ("generic" applies to all views). Values are action
# names as listed in the help view (h). "none" removes a binding.
# [keys.generic]
# "ctrl-d" = "move-half-page-down"
# "ctrl-u" = "move-half-page-up"
#
# [keys.main]
# "C" = "none"

# Confirmation for destructive actions can be disabled per action.
# [confirm]
# stash-drop = false

# User commands. argv is executed without a shell. Placeholders:
# %(commit) %(head) %(branch) %(remote) %(tag) %(stash) %(file) %(lineno)
# %(directory) %(repo:worktree) %(worktree)
# output: status | pager | none | suspend
#
# [[commands]]
# key     = "F"
# view    = "main"
# argv    = ["git", "fetch", "--all", "--prune"]
# confirm = false
# output  = "status"
`
