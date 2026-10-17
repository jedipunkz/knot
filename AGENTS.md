# AGENTS.md

Instructions for AI coding agents (Claude Code, OpenCode, Codex).

## Design document

[DESIGN.md](DESIGN.md) is knot's canonical design document, mirrored from
[issue #1](https://github.com/jedipunkz/knot/issues/1). Read it before changing
the architecture, views, keybindings, configuration, themes or worktree
behavior, and update it in the same change when the design moves.

Non-negotiables from the design:

- Default keybindings stay tig-compatible (DESIGN.md section 6).
- Views never use raw colors; every color goes through a semantic theme role
  (section 8).
- Git is always executed as an argv slice, never through a shell (section 9.4).
- Keep the view list (section 6.1), the action registry (section 6) and the
  TOML schema (section 9.3) in sync with DESIGN.md; the help view is generated
  from the live keymap.
- New view-side entry points are interfaces on `view.View` / `view.Env`
  (section 5.1); watch cross-package imports (`view` does not import `app`).

## Workflow

- Commit messages: Conventional Commits prefix + imperative subject, in English
  (feat / fix / docs / refactor / test / chore / build / ci).
- Verify before committing: `gofmt -l .` (no output), `go vet ./...`,
  `go test ./...`. Do not skip failures with `--no-verify`.
- Package responsibilities are in DESIGN.md section 5; view groups stay
  one-file-per-group under `internal/view`.
