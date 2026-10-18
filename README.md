# knot

A terminal UI for Git with the views and key bindings of
[tig](https://jonas.github.io/tig/), plus:

- `git worktree` view (`w`): list, switch, add, remove, lock, prune
- 8 built-in themes and user themes (`T` to pick)
- one TOML config file: `~/.config/knot/knot.toml`

## Install

```sh
brew tap jedipunkz/knot
brew install jedipunkz/knot/knot   # plain `knot` is the Knot DNS server
```

or `go install github.com/jedipunkz/knot/cmd/knot@latest` (Go 1.27.2+).

Requires git 2.36 or newer.

## Usage

```sh
knot                 # commit list
knot status          # also: log, show, blame, grep, refs, stash, worktree
git log -p | knot    # pager
```

Press `h` in knot to see all keys.

| Key | View | Key | View |
|---|---|---|---|
| `m` | main (commits) | `s` | status |
| `d` | diff | `c` | stage |
| `l` | log | `r` | refs |
| `t` | tree | `y` | stash |
| `b` | blame | `g` | grep |
| `w` | worktree | `T` | theme picker |

`j` / `k` move, `Enter` opens, `q` closes, `Q` quits, `/` searches.
In the main view, `Ctrl-P` on the first row opens the fuzzy filter.

## Worktrees

| Key | Action |
|---|---|
| `Enter` | switch to the worktree |
| `a` | add |
| `!` | remove |
| `L` | lock / unlock |
| `P` | prune (with a dry-run preview) |
| `o` | open a shell in the worktree |

## Configuration

knot reads the first file found and never writes it:

1. `--config <file>`
2. `$KNOT_CONFIG`
3. `$XDG_CONFIG_HOME/knot/knot.toml`
4. `~/.config/knot/knot.toml`

```sh
knot --print-default-config   # print the commented file with all defaults
knot --check-config           # validate the file, exit non-zero on errors
```

Unknown keys are reported as warnings; invalid values are errors and fall
back to their defaults. Problems are shown in the status bar at startup.

### Settings

All keys with their defaults:

```toml
theme = "default"                # built-in or user theme name, or "auto"

[theme-auto]                     # used when theme = "auto"
dark  = "dark"                   # theme on a dark terminal background
light = "light"                  # theme on a light terminal background

[ui]
line-graphics     = "utf-8"      # utf-8 | ascii | default
split-direction   = "horizontal" # horizontal | vertical | auto (vertical on wide terminals)
split-ratio       = 0.66         # child view share of a split, 0.1 - 0.9
show-line-numbers = false        # `#` toggles
mouse             = true         # wheel scrolls, click selects and opens
tab-size          = 8            # 1 - 16
title-overflow    = 50           # highlight subjects longer than this, `$` toggles
keybar            = true         # key hints above the status bar

[main]
columns          = ["date", "author", "commit-id", "graph", "refs", "subject"]
date-format      = "absolute"    # off | absolute | relative | relative-compact | custom
date-custom      = "%Y-%m-%d %H:%M"  # strftime subset, when date-format = "custom"
author           = "full"        # off | full | abbreviated | email | email-user
commit-id-length = 7             # 4 - 40
graph            = true

[diff]
context           = 3            # lines of context around hunks
ignore-whitespace = false
highlight         = "line"       # off | line | word

[blame]
columns = ["commit-id", "author", "date", "line-number"]

[worktree]
on-switch               = "main" # view opened after switching worktrees
show-dirty              = true   # dirty flag in the worktree view; false skips the check
dirty-check-concurrency = 4      # parallel dirty checks, at least 1
open-command            = ["$SHELL"]  # argv run with cwd = worktree path (`o`)

[git]
binary   = "git"
log-args = []                    # extra args appended to git log in the main and log views
```

`main.columns` allows `date`, `author`, `commit-id`, `graph`, `refs`,
`subject`; `blame.columns` allows `commit-id`, `author`, `date`,
`line-number`.

### Keys

Bindings are per view and `[keys.generic]` applies to every view. Values
are action names as shown in the help view (`h`), or:

- `"none"` removes a binding
- `"command:<n>"` runs the n-th `[[commands]]` entry

Key names are single characters (case sensitive), modified keys
(`"ctrl-d"`, `"<Ctrl-L>"`), and `"enter"`, `"space"`, `"tab"`, `"esc"`,
`"backspace"`, `"up"`, `"down"`, `"left"`, `"right"`, `"pgup"`, `"pgdown"`,
`"home"`, `"end"`, `"insert"`, `"delete"`, `"f1"`. Views: `main`, `diff`,
`log`, `reflog`, `tree`, `blob`, `blame`, `refs`, `status`, `stage`,
`stash`, `grep`, `pager`, `help`, `worktree`.

```toml
[keys.generic]
"ctrl-d" = "move-half-page-down"
"ctrl-u" = "move-half-page-up"

[keys.main]
"C" = "none"          # disable cherry-pick
"F" = "command:0"     # run the first [[commands]] entry
```

### Confirmation

These actions ask for confirmation by default; set to `false` to skip it:

```toml
[confirm]
# status-revert   = false   # discard changes, delete an untracked file
# cherry-pick     = false
# checkout        = false
# branch-delete   = false   # git branch -d, unmerged branches survive
# stash-apply     = false
# stash-pop       = false
# stash-drop      = false
# worktree-remove = false
# worktree-prune  = false
```

### Commands

```toml
[[commands]]
key     = "F"                                  # required
view    = "main"                               # optional, default: generic
argv    = ["git", "fetch", "--all", "--prune"] # required
confirm = false                                # ask before running
output  = "status"                             # status | pager | none | suspend
```

`argv` runs without a shell: environment variables are expanded first, then
placeholders are substituted per argument, so values such as branch names
are never re-interpreted. Placeholders: `%(commit)`, `%(head)`,
`%(branch)`, `%(remote)`, `%(tag)`, `%(stash)`, `%(file)`, `%(lineno)`,
`%(directory)`, `%(repo:worktree)`, `%(worktree)`; unknown ones are left
as they are.

`output`: `status` shows the last output line in the status bar, `pager`
opens the output in the pager view, `none` runs silently, `suspend` runs
the command with the terminal. `status`, `none` and `suspend` reload the
views afterwards.

Built-in themes: `default`, `tig-classic`, `dark`, `light`, `solarized-dark`,
`solarized-light`, `gruvbox`, `catppuccin-mocha`. User themes go in
`~/.config/knot/themes/<name>.toml`.

## Development

```sh
make check
```
