# knot

knot is a terminal UI for Git written in Go. It keeps the views and default
key bindings of [tig](https://jonas.github.io/tig/), and adds:

- `git worktree` support: a worktree view (`w`), switching between worktrees,
  and add / remove / lock / prune
- themes: 8 built-in themes, user themes, live theme picker (`T`)
- one TOML configuration file: `~/.config/knot/knot.toml`

The design is described in [#1](https://github.com/jedipunkz/knot/issues/1).

## Requirements

- git 2.36 or newer (knot runs the `git` command)
- Go 1.27.2 or newer to build

## Install

```sh
go install github.com/jedipunkz/knot/cmd/knot@latest
```

## Usage

```
knot [options] [revisions] [--] [paths]   # main view
knot log    [options] [revisions] [--] [paths]
knot show   [options] [revisions] [--] [paths]
knot reflog [options] [revisions]
knot blame  [options] [rev] [--] path
knot grep   [options] [pattern]
knot refs | stash | status | worktree
git log -p | knot                          # pager mode
```

Options: `+<n>` (select line n), `-C <path>`, `--config <file>`,
`--theme <name>`, `--print-default-config`, `--check-config`, `-v`, `-h`.
Other options such as `--all` are passed to git.

## Views and keys

Press `h` in knot for the full list of the active key bindings.

| Key | View | Key | View |
|---|---|---|---|
| `m` | main (commits) | `r` | refs |
| `d` | diff | `s` | status |
| `l` | log | `c` | stage |
| `t` | tree | `y` | stash |
| `f` | blob | `g` | grep |
| `b` | blame | `p` | pager |
| `h` | help | `w` | worktree (knot only) |

Common keys follow tig: `j`/`k`, `Up`/`Down` and `Ctrl-N`/`Ctrl-P` move, `Enter` opens, `q` closes the view,
`Q` quits, `Tab` switches the focus in a split, `/` `?` `n` `N` search,
`:` opens the prompt (`:42` jumps to a line, `:!git log -1` shows command
output), `#` `D` `A` `G` `~` `F` `W` `X` `%` `$` toggle display options,
`o` opens the option menu, `T` opens the theme picker.

Fuzzy filter (main view): a filter bar sits at the top of the main view at all
times, showing the live `<matches>/<loaded>` count. Pressing `Ctrl-P` (or `Up`)
while the cursor is on the first row focuses it — like tig, where `Ctrl-P` at
the top was unused. Typing fuzzy-filters the commit list live on
the commit subject, ref names and author, and by commit ID prefix; tokens
separated by `Space` are ANDed. `Enter`, `Ctrl-N` or `Down` return to the list
with the filter kept, `Esc` in the bar clears it and `Esc` in the filtered list
clears the filter while staying on the selected commit. The revision graph and
the "Unstaged / Staged changes" rows are hidden while a filter is active. The
`filter` action (unbound by default) opens the bar from any row and can be
bound in `knot.toml`; a spinner animates in the title bar while content loads.

Staging: in the status view `u` stages / unstages a file (on a section header:
all files of the section), `!` discards changes, `C` commits, `M` runs
`git mergetool`. In the stage view `u` stages the chunk under the cursor,
`1` stages a single line and `@` jumps to the next chunk.

Destructive actions (discard, delete branch, drop / pop / apply stash,
cherry-pick, checkout, remove / prune worktree) ask for confirmation.

## Worktrees

`w` lists the worktrees of the repository with their branch, HEAD and flags
(`dirty`, `locked`, `prunable`, ahead / behind). `*` marks the active
worktree and `@` the main worktree.

| Key | Action |
|---|---|
| `Enter` | switch to the worktree: all views are closed and the `[worktree] on-switch` view (default: main) opens with the cursor on HEAD |
| `a` | add a worktree (prompts for the path and an existing or new branch) |
| `!` | remove the worktree (a dirty worktree needs a second confirmation for `--force`) |
| `L` | lock / unlock |
| `P` | prune (shows a `--dry-run` preview first) |
| `o` | run `[worktree] open-command` (default `$SHELL`) in the worktree |

The refs view marks branches checked out in another worktree with `+` and
refuses to check them out. The main view shows them as `{wt:<name>}`.

knot cannot change the directory of the shell that started it; after
quitting, the shell is still in the original directory.

## Configuration

knot reads the first file found of `--config <file>`, `$KNOT_CONFIG`,
`$XDG_CONFIG_HOME/knot/knot.toml` and `~/.config/knot/knot.toml`. It never
writes the file. Print a commented file with all defaults:

```sh
knot --print-default-config > ~/.config/knot/knot.toml
knot --check-config
```

Example:

```toml
theme = "catppuccin-mocha"

[main]
date-format = "relative"
author = "abbreviated"

[worktree]
on-switch = "status"

[keys.generic]
"ctrl-d" = "move-half-page-down"
"ctrl-u" = "move-half-page-up"

[keys.main]
"C" = "none"          # disable cherry-pick

[confirm]
stash-drop = false    # do not ask before dropping a stash

[[commands]]
key     = "F"
view    = "main"
argv    = ["git", "fetch", "--all", "--prune"]
output  = "status"    # status | pager | none | suspend
```

User commands run without a shell. Placeholders (`%(commit)`, `%(branch)`,
`%(file)`, `%(lineno)`, `%(worktree)`, ...) are substituted per argument
after environment variables are expanded, so values such as branch names are
never interpreted.

## Themes

Built-in themes: `default`, `tig-classic`, `dark`, `light`,
`solarized-dark`, `solarized-light`, `gruvbox`, `catppuccin-mocha`.
`theme = "auto"` picks `[theme-auto] dark` or `light` from the terminal
background. `NO_COLOR` disables colors.

A user theme is `~/.config/knot/themes/<name>.toml` (next to the
configuration file in use):

```toml
extends = "gruvbox"

[styles]
diff-add = { fg = "#a6e3a1", attrs = ["bold"] }
cursor   = { fg = "black", bg = "214" }
```

Colors are `#rrggbb`, `#rgb`, a 256-color index or an ANSI name (`green`,
`bright-black`, ...); attributes are `bold`, `dim`, `italic`, `underline`,
`reverse` and `strikethrough`. Roles are listed in
[internal/theme/theme.go](internal/theme/theme.go).

## Development

```sh
go vet ./...
go test -race ./...
```
