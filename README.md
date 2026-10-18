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

```sh
knot --print-default-config > ~/.config/knot/knot.toml   # all options, commented
knot --check-config
```

```toml
theme = "catppuccin-mocha"   # or "auto", "gruvbox", "solarized-dark", ...

[keys.generic]
"ctrl-d" = "move-half-page-down"
```

Built-in themes: `default`, `tig-classic`, `dark`, `light`, `solarized-dark`,
`solarized-light`, `gruvbox`, `catppuccin-mocha`. User themes go in
`~/.config/knot/themes/<name>.toml`.

## Development

See [DESIGN.md](DESIGN.md) and [AGENTS.md](AGENTS.md).

```sh
make check
```
