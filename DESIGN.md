# Design: knot — a tig-compatible TUI for Git, written in Go

> Status: M1–M6 are implemented on `main`. This document describes the implemented design; differences from the first draft are listed in section 13.

## 1. Summary

`knot` is a terminal UI for browsing and operating on Git repositories, modeled on [tig](https://jonas.github.io/tig/). It keeps tig's views and default keybindings so tig users can switch with zero relearning, and adds three features tig does not have:

1. First-class `git worktree` support
2. Themes (built-in + user-defined)
3. A single TOML config file at `~/.config/knot/knot.toml`

## 2. Goals / Non-goals

### Goals

- Feature parity with tig's core views (main, diff, log, reflog, tree, blob, blame, refs, status, stage, stash, grep, pager, help).
- Default keybindings identical to tig's defaults (section 6).
- Worktree view and worktree-aware context switching (section 7).
- Theming with semantic color roles, 16/256/truecolor support, `NO_COLOR` support (section 8).
- All configuration in one TOML file, with every default overridable (section 9).
- Single static binary, Linux / macOS (Windows best-effort).

### Non-goals (initial release)

- Reading `~/.tigrc` (maybe later as an importer; see Open Questions).
- A built-in Git implementation. knot shells out to the `git` CLI.
- Interactive rebase editor, merge conflict editor (delegate to `git mergetool` / `$EDITOR` like tig).

## 3. Tech stack

| Concern | Choice | Reason |
|---|---|---|
| Language | Go 1.27.2 | Requirement; single binary |
| TUI framework | [Bubble Tea v2](https://github.com/charmbracelet/bubbletea) (`charm.land/bubbletea/v2` v2.1.0) + [Lip Gloss v2](https://github.com/charmbracelet/lipgloss) (`charm.land/lipgloss/v2` v2.0.6) | Elm-style update loop fits a multi-view app; the renderer downgrades colors to the terminal's profile (truecolor → 256 → 16 → none). Bubbles is not used: the list, prompt and confirm widgets are small enough to own |
| Git access | `git` CLI via `os/exec` with machine-readable formats (`--porcelain=v2 -z`, `--format` with `%x1f` field separators) | Full compatibility with user's git config, hooks, worktrees, sparse checkout, partial clone; go-git has incomplete worktree support |
| Config parsing | [BurntSushi/toml](https://github.com/BurntSushi/toml) v1.6.0 | Mature, reports undecoded (unknown) keys |
| CLI parsing | Standard library, hand-written parser | git-style arguments (`+<n>`, revisions and options passed through to git, `--`) do not fit `flag`; no dependency needed |

Git commands are always executed with an argv slice (no shell), with `-C <worktree path>`, `--no-pager`, `-c color.ui=never`, `-c core.quotepath=off`, `GIT_TERMINAL_PROMPT=0`, and with `GIT_OPTIONAL_LOCKS=0` for read-only commands so knot never takes the index lock while browsing.

knot requires git ≥ 2.36 (`git worktree list --porcelain -z`) and refuses to start with an older git.

## 4. Command-line interface

Mirrors tig:

```
knot [options] [revisions] [--] [paths]   # main view
knot log    [options] [revisions] [--] [paths]
knot show   [options] [revisions] [--] [paths]
knot reflog [options] [revisions]
knot blame  [options] [rev] [--] path
knot grep   [options] [pattern]
knot refs   [options]
knot stash  [options]
knot status
knot worktree                              # new: open worktree view
git log -p | knot                          # pager mode (stdin; keys are read from /dev/tty)
```

Options:

| Option | Meaning |
|---|---|
| `+<n>` | Open the first view with line `n` selected |
| `-C <path>` | Run as if started in `<path>` |
| `--config <file>` | New: use an alternative config file |
| `--theme <name>` | New: override theme for this run |
| `--print-default-config` | New: print the fully commented default configuration |
| `--check-config` | New: validate the configuration file, print problems, exit non-zero if any |
| `-v`, `--version` / `-h`, `--help` | Version / help |

Any other option (e.g. `--all`) is passed to git.

## 5. Architecture

```
cmd/knot/            CLI parsing, config/theme/keymap setup, git version check, initial view
internal/app/        root Bubble Tea model: view stack, split layout, title bars, status bar,
                     prompt, confirm dialog, action dispatch, mouse, user commands
internal/view/       all views (one file per view group) sharing Base, Env, rendering and search
internal/git/        typed wrappers around the git CLI, streaming, parsers, patch builder
internal/graph/      one-row-per-commit revision graph computed from parents (like tig)
internal/keymap/     action registry, default tig keymap, user overrides, key name normalization
internal/config/     knot.toml lookup, defaults, validation, commented default file
internal/theme/      color roles, built-in themes, user theme loading with extends
```

### 5.1 Core abstractions

```go
// A View is one tig-style screen.
type View interface {
    B() *Base                                // cursor, offset, search, loading state, view ID
    Name() string                            // "main", "diff", ...
    Title(env *Env) string                   // title bar text
    Len() int
    Row(env *Env, i int) []Seg               // styled segments; rendered with the current theme
    Load(env *Env) tea.Cmd                   // start (re)loading, cancellable
    HandleMsg(env *Env, msg tea.Msg) tea.Cmd // async results; stale results from superseded loads are dropped via a per-view load generation
    Enter(env *Env) tea.Cmd                  // open the selected line
    Do(env *Env, a keymap.Action) (tea.Cmd, bool) // view-specific actions
    Selection(env *Env) Selection            // commit / ref / file / line / worktree under cursor
}

// Env is what every view runs against. Switching worktree replaces Loc, Head and Refs.
type Env struct {
    Repo  *git.Repo        // common dir, shared across worktrees
    Loc   git.Location     // git dir + worktree root of the active worktree
    Head  git.Head
    Refs  []git.Ref        // + Decor: commit → refs
    Cfg   *config.Config
    Theme *theme.Theme
    Keys  *keymap.Keymap
    Opts  *Options         // runtime toggles (D, A, G, ~, F, W, X, %, $, #, diff context)
    // ...
}
```

- **View stack**: like tig, opening a view pushes it; `q` closes the top view (the child when split); `Q` quits. `Enter` on a commit opens a child view in a split (`[ui] split-direction` horizontal / vertical / auto, `split-ratio`). Pressing a view key for a list view that is already open (main, log, refs, status, stash, reflog, help, worktree, grep, pager) returns to it and closes the views above it.
- **Loading**: every load runs in a goroutine with a `context.Context`; `z` cancels all loads. The main view streams `git log` output in batches of 500 commits so the first screen renders immediately; the log and diff views stream their text output through the same ingestion machinery in fixed line batches (the commit-header restyle of a diff runs once, on completion). Smaller text outputs (pager, blob, stage, status) render when their command finishes.
- **Action dispatch**: keys map to named actions (`view-main`, `move-down`, `status-update`, ...). Resolution order: view-specific binding → generic binding. The focused view's `Do` gets the action first, then the app handles generic actions. Rendering uses roles, so theme changes and display toggles apply without reloading.
- **Messages from views to the app**: `OpenMsg`, `StatusMsg`, `ConfirmMsg`, `PromptMsg`, `ReloadMsg` (reload refs and all open views after a git write), `CloseMsg`, `SwitchWorktreeMsg`, `PreviewThemeMsg`, `ExecMsg` (run a process with the terminal, then reload).

### 5.2 Git layer (`internal/git`)

| Data | Command |
|---|---|
| Commit list + graph | `git log --format=%H%x1f%P%x1f%an%x1f%ae%x1f%at%x1f%s [--topo-order] [log-args] [revs] [-- paths]` (graph computed in Go from parents, like tig; `--topo-order` is added while the graph is shown unless `log-args` sets an order, like tig's `commit-order=auto`) |
| Unstaged / staged pseudo commits | `git status --porcelain=v2 -z` (rows at the top of the main view, like tig) |
| Refs | `git for-each-ref` with `%(worktreepath)`, `%(upstream:track)`, peeled tags |
| Status | `git status --porcelain=v2 -z --branch --untracked-files=all` |
| Diff / show | `git show --patch-with-stat --format=fuller -M --root -U<n> [-w]`, `git diff [--cached] --patch-with-stat` |
| Stage chunk / line | `git diff [--cached] -U<n> -- <file>` → patch built in Go → `git apply --cached [--reverse] -` |
| Stage / unstage file | `git add -A -- <file>` / `git reset -q HEAD -- <file>` (`git rm --cached` before the first commit) |
| Blame | `git blame --porcelain [rev] -- <file>` |
| Stash | `git stash list --format=...`, `git stash show -p --stat` |
| Reflog | `git reflog show --format=...` |
| Grep | `git grep -n -z -I --no-color` (exit code 1 = no match) |
| Tree / blob | `git ls-tree -z <rev>:<dir>`, `git cat-file -p <rev>:<path>`; worktree files are read from disk |
| Worktrees | `git worktree list --porcelain -z`; subjects via `git log --no-walk=unsorted`; add / remove / lock / unlock / prune |

## 6. Views and default keybindings (tig-compatible)

### 6.1 Views

| View | Key | Content |
|---|---|---|
| main | `m` | One line per commit: date, author, commit ID, graph, refs (branches/tags/remotes/worktrees), subject; "Unstaged changes" / "Staged changes" rows on top |
| diff | `d` | Commit message, diffstat and patch; or working tree changes |
| log | `l` | Full commit messages with diffstat per revision |
| reflog | — (`knot reflog`) | Reflog entries |
| tree | `t` | Directory listing at a revision; navigate into dirs / open blobs |
| blob | `f` | File contents at a revision (or in the worktree) |
| blame | `b` | File contents annotated with commit, author, date, line number |
| refs | `r` | Branches, remotes, tags with upstream tracking |
| status | `s` | Staged / unstaged / untracked files |
| stage | `c` | Diff of a staged/unstaged file, or contents of an untracked file |
| stash | `y` | Stash list |
| grep | `g` | Files and matching lines for a pattern (prompts for the pattern) |
| pager | `p` | stdin or output of a prompt command |
| help | `h` | Key binding reference (generated from the live keymap, including user commands) |
| **worktree** (new) | **`w`** | Worktrees of the repository (section 7) |
| options (new view for tig's option menu) | `o` | Display options; `Enter` changes the value |
| theme (new) | `T` | Theme picker (section 8.3) |

### 6.2 View switching / manipulation

| Key | Action |
|---|---|
| `m d l p t f g b r y h s c` | Switch to view (table above) |
| `w` | New: worktree view |
| `q` | Close current view (the child in a split); quit if last |
| `Q`, `Ctrl-C` | Quit |
| `Enter` | Open selection (e.g. commit → diff in split); in diff view, scroll down one line (on a diffstat line: jump to that file) |
| `Tab` | Focus the other view in a split |
| `R` | Reload refs and the current view |
| `O` | Maximize current view |
| `Up` / `Down` in the child of a split | Move in parent view and update child view |
| `,` | Go to parent (parent dir in tree, parent commit in blame) |

### 6.3 Cursor navigation / scrolling / search

| Key | Action |
|---|---|
| `k` / `j`, `Up` / `Down` | Move up / down one line |
| `PgUp`, `-` / `PgDown`, `Space` | Page up / down |
| `Home` / `End` | First / last line |
| `Insert` / `Delete` | Scroll one line up / down |
| `Left` / `Right` | Scroll one column left / right |
| `\|` | Scroll to first column |
| `/` / `?` | Search forward / backward (regexp, smart case) |
| `n` / `N` | Next / previous match |
| `:` | Command prompt: `:<n>` jumps to line n, `:!<cmd>` runs a command (no shell) into the pager, `:<action-name>` runs an action, `:q` quits |

Mouse (`[ui] mouse = true`): wheel scrolls the view under the pointer, click selects a line, clicking the selected line opens it.

### 6.4 View-specific actions

Verified against tig's default `tigrc`.

| Key | View | Action |
|---|---|---|
| `u` | status | Stage / unstage file; on a section header: all files of the section |
| `u` | stage | Stage / unstage the chunk under the cursor (outside a chunk: the whole file) |
| `1` | stage | Stage / unstage single line |
| `@` | stage | Jump to next chunk |
| `[` / `]` | diff / stage | Decrease / increase diff context |
| `!` | status / stage | Discard unstaged changes of the file; delete an untracked file (confirm) |
| `M` | status | Resolve conflict with `git mergetool` |
| `C` | status / stage | `git commit` (TUI suspended) |
| `C` | main | Cherry-pick commit (confirm) |
| `C` | refs | Checkout branch / tag (confirm) |
| `!` | refs | Delete branch with `git branch -d` (confirm) |
| `A` / `P` / `!` | stash | Apply / pop / drop stash (confirm) |
| `e` | diff / blob / blame / stage / grep | Open file in `$VISUAL` / `$EDITOR` (`vi` if unset) at line (`+<line>`) |

### 6.5 Display toggles / misc

| Key | Action |
|---|---|
| `#` | Toggle line numbers |
| `D` | Cycle date display (off / absolute / relative / relative-compact / custom) |
| `A` | Cycle author display (off / full / abbreviated / email / email-user) — generic; overridden by stash `A` |
| `G` | Toggle revision graph |
| `~` | Toggle line graphics (utf-8 / ascii) |
| `F` | Toggle ref display |
| `W` | Toggle ignore whitespace in diff |
| `X` | Toggle commit ID column |
| `%` | Toggle file filtering in diff |
| `$` | Toggle commit title overflow highlight (`[ui] title-overflow`, default 0 = off) |
| `H` | Go to HEAD commit |
| `o` | Option menu |
| `z` | Stop all background loading |
| `v` | Show version |
| `Ctrl-L` | Redraw |
| **`T`** | New: open theme picker (live preview) |

## 7. Feature: git worktree support

### 7.1 Worktree view (`w`, `knot worktree`)

Source: `git worktree list --porcelain -z`, plus a dirty check per worktree (`git -C <path> status --porcelain=v2 -z --untracked-files=no`, run concurrently with `[worktree] dirty-check-concurrency`, cancellable, disabled with `show-dirty = false`).

Columns:

| Column | Example |
|---|---|
| Marker | `*` current worktree, `@` main worktree (when not current) |
| Path | `~/src/knot-feature-x` (home-abbreviated, long paths keep their end) |
| Branch | `feature/x` / `(detached)` / `(bare)` |
| HEAD | short SHA + subject |
| Flags | `dirty`, `locked`, `prunable`, `ahead/behind` upstream |

Actions:

| Key | Action | Confirm |
|---|---|---|
| `Enter` | Switch to the selected worktree and open the `on-switch` view (default: main view; see 7.2) | no |
| `a` | Add worktree: prompt for path, then branch (existing → checkout, new → `-b`, empty → `--detach`) → `git worktree add` | no |
| `!` | Remove worktree → `git worktree remove`; refused for the main and the active worktree; a dirty worktree needs a second confirmation for `--force` | yes |
| `L` | Lock (prompt for an optional reason) / unlock | no |
| `P` | Prune → shows `git worktree prune --dry-run --verbose` output in the confirmation, then prunes | yes |
| `o` | Run `[worktree] open-command` (default `$SHELL`) with cwd = worktree, TUI suspended | no |

### 7.2 Switching behavior (`Enter` in worktree view)

1. Replace the active location (`Env.Loc`, `Head`, `Refs`) with the selected worktree (all subsequent git commands run with `-C <path>`). Command-line revisions and paths are dropped.
2. Close every open view (the whole view stack). Views loaded for the previous worktree are discarded so stale content is never shown; `q` after switching quits instead of returning to the old worktree's views.
3. Open the view configured by `[worktree] on-switch` (default: `main`), with the cursor on the selected worktree's HEAD commit. Supported values: main, status, refs, log, stash, reflog, tree, worktree, help, diff (unstaged changes), grep; other view names open main.

Edge cases:

| Case | Behavior |
|---|---|
| Selected worktree is the current one | No reload of the location; just open the `on-switch` view (same stack reset as above) |
| Worktree is prunable (directory missing) | Do not switch; show an error in the status bar |
| Worktree is bare (main worktree of a bare repo) | Do not switch; show an error in the status bar |

The switch only affects knot's internal context. knot cannot change the parent shell's working directory; after quitting, the shell stays where knot was started (see Open Questions for an optional shell integration).

### 7.3 Worktree awareness in other views

- Status bar always shows the active worktree name and branch (`demo-wt [topic]`, or `[detached <sha>]`).
- refs view: branches checked out in another worktree are marked `+` (like `git branch`) and show `{wt:<path>}`; checkout of such a branch is blocked with an explanation instead of a git error; deleting it is refused.
- main view: ref decoration `{wt:<name>}` after branches checked out in another worktree.
- `knot -C <worktree>` and starting knot inside a linked worktree work transparently (`git rev-parse --git-common-dir`).

## 8. Feature: themes

### 8.1 Model

Themes map **semantic color roles** to styles. Views never use raw colors.

Roles:
`default`, `cursor`, `title-focus`, `title-blur`, `status-bar`, `line-number`, `date`, `author`, `commit-id`, `graph-1`..`graph-6`, `ref-head`, `ref-branch`, `ref-remote`, `ref-tag`, `ref-stash`, `ref-worktree`, `diff-add`, `diff-del`, `diff-add-highlight`, `diff-del-highlight`, `diff-hunk`, `diff-header`, `diff-meta`, `stat-add`, `stat-del`, `status-staged`, `status-unstaged`, `status-untracked`, `section`, `search-match`, `prompt`, `error`, `warning`, `title-overflow`, `directory`, `marker`, `author-dim`, `date-dim`, `commit-id-dim`, `ref-dim`.

The `*-dim` roles are the dark variants of a field's value color (value color + `dim` attribute by default); they are used for the field labels of the diff view header.

A style is:

```toml
fg = "#a6e3a1"     # #rrggbb / #rgb (truecolor), 0-255 (256-color), ANSI name ("green", "bright-black"), "default"
bg = "default"
attrs = ["bold"]   # bold, dim, italic, underline, reverse, strikethrough
```

### 8.2 Built-in themes

`default` (ANSI 16 colors, follows the terminal palette, reverse-video cursor), `tig-classic` (tig's colors), `dark`, `light`, `solarized-dark`, `solarized-light`, `gruvbox`, `catppuccin-mocha`.

### 8.3 User themes

- Location: `<config dir>/themes/<name>.toml` (the directory of the config file in use, else `$XDG_CONFIG_HOME/knot` or `~/.config/knot`). A user theme with a built-in name overrides it.
- Format:

  ```toml
  extends = "gruvbox"          # optional: built-in or user theme; cycles are rejected

  [styles]
  diff-add = { fg = "#a6e3a1", attrs = ["bold"] }
  cursor   = { fg = "black", bg = "214" }
  ```

  Roles not set fall back to the extended theme, then to `default`. Unknown roles and keys are errors.
- Theme selection: `--theme` flag > `theme` in `knot.toml` > `default`.
- `T` opens a picker with live preview; `Enter` applies it for the session, `q` restores the previous theme. The selection is not written to disk.

### 8.4 Terminal capability handling

- The color profile is detected by Bubble Tea; hex colors are downgraded to 256 or 16 colors automatically.
- `NO_COLOR` set → no colors, attributes only (the cursor stays visible through reverse / bold).
- `theme = "auto"` chooses `[theme-auto] dark` or `light` from the terminal background color reported at startup.

## 9. Feature: configuration file `~/.config/knot/knot.toml`

### 9.1 Lookup order (first found wins)

1. `--config <file>` (an error if it does not exist)
2. `$KNOT_CONFIG`
3. `$XDG_CONFIG_HOME/knot/knot.toml`
4. `~/.config/knot/knot.toml`

No file → built-in defaults. knot never creates or rewrites the config file by itself; `knot --print-default-config` prints a fully commented default file (every value equals the built-in default, checked by a test).

### 9.2 Validation

- TOML syntax errors stop knot with `file:line`.
- Unknown keys → warning (not fatal) with `file:line`.
- Invalid values (out-of-range numbers, unknown enum values, unknown columns or views, bad user commands) → error with `file:line`, then fall back to the default for that key.
- Unknown actions or key names in `[keys]` → error, the binding is skipped.
- Problems are shown in the status bar at startup; `knot --check-config` lists all of them and exits non-zero.
- Invalid themes: the default theme is used with a warning (an invalid `--theme` is fatal).

### 9.3 Schema

Defaults follow tig.

```toml
theme = "default"

[theme-auto]
dark  = "dark"
light = "light"

[ui]
line-graphics     = "utf-8"      # utf-8 | ascii | default
split-direction   = "horizontal" # horizontal | vertical | auto (vertical when ≥ 160 columns)
split-ratio       = 0.66         # child view size, 0.1 - 0.9
show-line-numbers = false
mouse             = true
tab-size          = 8
title-overflow    = 0            # 0 = off; positive: highlight longer subjects
keybar            = true         # curated key hints above the status bar

[main]
columns          = ["date", "author", "commit-id", "graph", "refs", "subject"]
date-format      = "absolute"    # off | absolute | relative | relative-compact | custom
date-custom      = "%Y-%m-%d %H:%M"
author           = "full"        # off | full | abbreviated | email | email-user
commit-id-length = 7
graph            = true

[diff]
context           = 3
ignore-whitespace = false
highlight         = "line"       # off | line | word

[blame]
columns = ["commit-id", "author", "date", "line-number"]

[worktree]
on-switch               = "main"
show-dirty              = true
dirty-check-concurrency = 4
open-command            = ["$SHELL"]  # argv; executed with cwd = worktree path

[git]
binary   = "git"
log-args = []

# Keybindings: per-view tables, plus "generic". Values are action names
# (listed in the help view). "none" removes a binding. Key names:
# single characters (case sensitive), "ctrl-d" / "ctrl+d", "<Ctrl-L>", "Enter",
# "Space", "Tab", "Esc", "PgUp", "PgDown", "Home", "End", "Insert", "Delete", "F1".
[keys.generic]
"ctrl-d" = "move-half-page-down"
"ctrl-u" = "move-half-page-up"

[keys.main]
"C" = "none"                   # unbind (disable cherry-pick)

# Confirmation of destructive actions can be disabled per action.
[confirm]
stash-drop = false

# User commands (tig's `bind ... !git ...` equivalent). view defaults to generic.
[[commands]]
key     = "F"
view    = "main"
argv    = ["git", "fetch", "--all", "--prune"]
confirm = false
output  = "status"             # status | pager | none | suspend

[[commands]]
key     = "R"
view    = "refs"
argv    = ["git", "rebase", "%(branch)"]
confirm = true
output  = "suspend"
```

Placeholders (tig-compatible names): `%(commit)`, `%(head)`, `%(branch)`, `%(remote)`, `%(tag)`, `%(stash)`, `%(file)`, `%(lineno)`, `%(directory)`, `%(repo:worktree)`, plus new `%(worktree)` (selected worktree path, else the active one). Unknown placeholders are left as they are.

Output modes: `status` shows the last output line in the status bar, `pager` opens the output in the pager view, `none` runs silently, `suspend` runs the command with the terminal. `status`, `none` and `suspend` reload the views afterwards.

### 9.4 Security considerations

- `argv` is executed directly (no shell). Environment variables (`$SHELL`) are expanded first, placeholders second, per argument, so commit messages / branch names / file names are never re-interpreted (neither by a shell nor by `$VAR` expansion).
- `:!<cmd>` in the prompt is split with quote handling but without a shell.
- Users who explicitly want a shell must write `argv = ["sh", "-c", "..."]`.
- Destructive built-in actions (discard changes, delete untracked file, delete branch, apply / pop / drop stash, cherry-pick, checkout, remove / prune worktree) confirm by default; confirmation can be disabled per action via `[confirm]`.
- Branch deletion uses `git branch -d`, so unmerged branches are not deleted.
- Paths read from the worktree (blob, stage) are checked to stay inside the worktree.
- Config files are only read from the locations in 9.1; knot never reads repo-local config that can execute commands (unlike `.tigrc` in a repo).

## 10. Testing strategy

- `internal/git`: tests against real `git` in `t.TempDir()` repositories (log streaming, refs with worktree paths and peeled tags, status with renames / untracked, linked / locked / prunable worktrees and add / remove / unlock / prune, stash, reflog, ls-tree, grep, blame, chunk and line staging / unstaging applied with `git apply`).
- `internal/graph`: linear and merge histories.
- `internal/config`: default file equals `Default()`, decoding, validation messages with line numbers, lookup order.
- `internal/theme`: all built-ins resolve every role, color parsing, `extends`, cycles, unknown roles, path-like names.
- `internal/keymap`: default keymap matches the tig tables in this issue, overrides, key name normalization.
- `internal/view`: date / author formatting, diff line references, word highlighting, diffstat coloring, navigation and search.
- `internal/app`: model-level tests that drive `Update` with key messages: worktree switch (stack reset, cursor on HEAD, status bar, `on-switch`, prunable refusal), split navigation, confirmation cancel / accept, checkout blocked for another worktree, user commands (placeholders, no re-expansion, pager output), prompt argument splitting.
- CI (GitHub Actions): gofmt, `go vet`, staticcheck, `go test -race` on ubuntu-latest and macos-latest.
- CI coverage gate (pull requests only): coverage profile from head and from the base commit (via `git worktree add`), per-package table posted as a sticky PR comment; the job fails when the total statements coverage drops more than 0.5 pp against base.
- CI performance gate: benchmarks for the filter hot path (`internal/view`: one filter pass over 500 rows, four `fuzzyMatch` calls per row) and graph construction (`internal/graph`: 1000 commits with merges) compared with `benchstat` head vs base (count 10), posted as a sticky PR comment.
- CI vulnerability scan: `govulncheck ./...` on pull requests and on a weekly schedule; `make bench` runs the same benchmarks locally.

## 11. Milestones

| # | Scope | Status |
|---|---|---|
| M1 | Skeleton: CLI, config loading, theme loading, keymap, view stack, status bar, help view | done |
| M2 | Read-only core: main (with graph), diff, log, refs, tree, blob | done |
| M3 | status + stage (stage/unstage file, chunk, line; commit via `$EDITOR`) | done |
| M4 | blame, stash, reflog, grep, pager (stdin + `:!cmd`) | done |
| M5 | worktree view and worktree-aware context switching | done |
| M6 | User commands, placeholders, theme picker, built-in theme set | done |
| M7 | Polish: mouse, performance on large repos (Linux kernel–sized log), release (goreleaser, Homebrew) | mouse done; benchmark base set (filter pass, graph build, tracked in CI), kernel-sized-log performance and release not started |

## 12. Open questions

Resolved:

1. Minimum supported git version → 2.36, checked at startup.
2. `w` for the worktree view → adopted.
4. Cobra vs standard `flag` → neither; a small hand-written parser (section 3).
5. Default split direction → `horizontal` like tig; `auto` is available.
6. Built-in theme list → the 8 themes in 8.2.

Open:

3. Provide a `.tigrc` → `knot.toml` importer (`knot --import-tigrc`)?
7. Shell integration for worktree switching: add an option that prints the last active worktree path on exit, so users can `cd "$(knot --print-worktree-on-exit)"` (or a shell function wrapping it)?
8. CI job against the minimum git version (2.36).
9. teatest golden-output tests for rendering (currently model-level tests only).
10. Stream blame / grep / log output like the main view for very large files and repositories.

## 13. Differences from the first draft

| Draft | Implemented | Reason |
|---|---|---|
| One package per view under `internal/view/<name>/`, plus `internal/ui/` | One `internal/view` package; graph in `internal/graph` | Views share `Base`, `Env`, rendering and search |
| `View` with `ID/Init/Update/Render/Actions/Selected`, `Context` | Interface and `Env` in 5.1 | Rows are segments rendered by the app, so themes and toggles apply without reloading |
| Streaming for `git log` and `git blame` | Streaming for the main, log and diff views | The main view is the large case; the log and diff text outputs are streamed in line batches (issue #13) |
| `git blame --incremental` | `git blame --porcelain` | Simpler parsing |
| Optional `[theme.auto]` | `theme = "auto"` + `[theme-auto]` | `theme = "..."` and `[theme.auto]` cannot coexist in TOML |
| Draft schema values (`date-format = "relative"`, `author = "abbreviated"`, `highlight = "word"`, `theme = "dark"`) | tig defaults (absolute, full, line, default) | The draft values were examples |
| main view worktree column | `{wt:<name>}` ref decoration | Covers 7.3 without a new column |
| `!` in refs = tig's `git branch -D` | `git branch -d` | Do not delete unmerged branches |
| `!` in stage reverts the chunk (tig) | Reverts the whole file | Simpler and confirmed by a dialog |
| Bubbles widgets | Own list / prompt / confirm | Small and fully controlled |
| Key bar | Not in the draft | Curated key hints per view above the status bar; keys are resolved from the live keymap so rebinding is reflected; `[ui] keybar = false` hides it |
| Diff header as raw `git show --format=fuller` text | Rebuilt as segmented theme-colored rows: commit, Refs (main-view ref decorations), Merge, Author, AuthorDate, Commit, CommitDate; labels in the `author-dim` / `date-dim` / `commit-id-dim` / `ref-dim` dark variants of their value's color, values in `commit-id` / author / `ref-*` / `date` roles | Consistent theming across views; issue #2 follow-up added the Refs row |
| Fuzzy filter bar | Not in the draft; designed and implemented (issue #2) | fzf-style narrowing of the main view |
| Worktree view opens with a focused filter bar | Not in the draft | Typing narrows the worktree list (path / branch / HEAD ID / subject); Enter / Ctrl-N / Down return to the list, which moves with the common navigation keys, and Ctrl-P on the first list row re-focuses the bar; the selected worktree's row is shown in the key bar area below the list (`view.InfoLine`) |
