package view

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	tea "charm.land/bubbletea/v2"

	"github.com/jedipunkz/knot/internal/git"
	"github.com/jedipunkz/knot/internal/keymap"
	"github.com/jedipunkz/knot/internal/theme"
)

type worktreeRow struct {
	wt      git.Worktree
	subject string
	track   string
	dirty   int // -1 unknown, 0 clean, 1 dirty
}

// WorktreeView lists the worktrees of the repository.
type WorktreeView struct {
	Base
	rows []worktreeRow
}

// NewWorktree creates a worktree view.
func NewWorktree() *WorktreeView { return &WorktreeView{} }

func (v *WorktreeView) Name() string          { return "worktree" }
func (v *WorktreeView) Title(env *Env) string { return "[worktree] " + env.Abbrev(env.Repo.CommonDir) }
func (v *WorktreeView) Len() int              { return len(v.rows) }

type worktreeListMsg struct {
	id   int
	rows []worktreeRow
	err  error
}

func (m worktreeListMsg) ViewID() int { return m.id }

type dirtyMsg struct {
	id    int
	dirty map[string]bool
}

func (m dirtyMsg) ViewID() int { return m.id }

func (v *WorktreeView) Load(env *Env) tea.Cmd {
	ctx := v.startLoad()
	r := env.Git()
	id := v.ID
	refs := env.Refs
	return func() tea.Msg {
		wts, err := git.ListWorktrees(ctx, r)
		if err != nil {
			return worktreeListMsg{id: id, err: err}
		}
		rows := make([]worktreeRow, len(wts))
		var hashes []string
		for i, wt := range wts {
			rows[i] = worktreeRow{wt: wt, dirty: -1}
			if wt.Head != "" {
				hashes = append(hashes, wt.Head)
			}
			for _, ref := range refs {
				if ref.Kind == git.RefBranch && ref.Short == wt.Branch {
					rows[i].track = ref.Track
				}
			}
		}
		subjects := map[string]string{}
		if len(hashes) > 0 {
			args := append([]string{"log", "--no-walk=unsorted", "--format=%H%x1f%s"}, hashes...)
			if out, err := r.Output(ctx, args...); err == nil {
				for _, l := range strings.Split(strings.TrimSpace(string(out)), "\n") {
					if h, s, ok := strings.Cut(l, "\x1f"); ok {
						subjects[h] = s
					}
				}
			}
		}
		for i := range rows {
			rows[i].subject = subjects[rows[i].wt.Head]
		}
		return worktreeListMsg{id: id, rows: rows}
	}
}

// dirtyCheck runs git status in every worktree with limited concurrency.
func (v *WorktreeView) dirtyCheck(env *Env) tea.Cmd {
	ctx := v.startLoad()
	id := v.ID
	var paths []string
	for _, r := range v.rows {
		if !r.wt.Bare && !r.wt.Prunable {
			paths = append(paths, r.wt.Path)
		}
	}
	limit := env.Cfg.Worktree.DirtyCheckConcurrency
	repo := env.Repo
	return func() tea.Msg {
		res := map[string]bool{}
		var mu sync.Mutex
		var wg sync.WaitGroup
		sem := make(chan struct{}, limit)
		for _, p := range paths {
			wg.Add(1)
			go func(p string) {
				defer wg.Done()
				select {
				case sem <- struct{}{}:
				case <-ctx.Done():
					return
				}
				defer func() { <-sem }()
				d, err := git.IsDirty(ctx, git.Runner{Binary: repo.Binary, Dir: p})
				if err == nil {
					mu.Lock()
					res[p] = d
					mu.Unlock()
				}
			}(p)
		}
		wg.Wait()
		return dirtyMsg{id: id, dirty: res}
	}
}

func (v *WorktreeView) HandleMsg(env *Env, msg tea.Msg) tea.Cmd {
	switch m := msg.(type) {
	case worktreeListMsg:
		v.finish(m.err)
		v.rows = m.rows
		v.Clamp(len(v.rows))
		if m.err == nil && env.Cfg.Worktree.ShowDirty {
			return v.dirtyCheck(env)
		}
	case dirtyMsg:
		v.finish(nil)
		for i := range v.rows {
			if d, ok := m.dirty[v.rows[i].wt.Path]; ok {
				v.rows[i].dirty = 0
				if d {
					v.rows[i].dirty = 1
				}
			}
		}
	}
	return nil
}

func (v *WorktreeView) isCurrent(env *Env, wt git.Worktree) bool {
	if env.Loc.TopLevel != "" {
		return samePath(wt.Path, env.Loc.TopLevel)
	}
	return wt.Bare
}

func samePath(a, b string) bool {
	ra, err1 := filepath.EvalSymlinks(a)
	rb, err2 := filepath.EvalSymlinks(b)
	if err1 == nil && err2 == nil {
		return ra == rb
	}
	return filepath.Clean(a) == filepath.Clean(b)
}

func (v *WorktreeView) Row(env *Env, i int) []Seg {
	r := v.rows[i]
	mark := "  "
	switch {
	case v.isCurrent(env, r.wt):
		mark = "* "
	case r.wt.Main:
		mark = "@ "
	}
	branch := r.wt.Branch
	switch {
	case r.wt.Bare:
		branch = "(bare)"
	case r.wt.Detached || branch == "":
		branch = "(detached)"
	}
	segs := []Seg{
		{theme.Marker, mark},
		{theme.Directory, FitTail(env.Abbrev(r.wt.Path), 40) + " "},
		{theme.RefBranch, Fit(branch, 24) + " "},
		{theme.CommitID, Fit(Short(r.wt.Head, env.Cfg.Main.CommitIDLength), env.Cfg.Main.CommitIDLength) + " "},
	}
	var flags []string
	if r.dirty == 1 {
		flags = append(flags, "dirty")
	}
	if r.wt.Locked {
		flags = append(flags, "locked")
	}
	if r.wt.Prunable {
		flags = append(flags, "prunable")
	}
	if r.track != "" {
		flags = append(flags, strings.Trim(r.track, "[]"))
	}
	if len(flags) > 0 {
		segs = append(segs, Seg{theme.Warning, "[" + strings.Join(flags, ", ") + "] "})
	}
	return append(segs, Seg{theme.Default, r.subject})
}

func (v *WorktreeView) current() *worktreeRow {
	if v.Cursor < len(v.rows) {
		return &v.rows[v.Cursor]
	}
	return nil
}

// Enter switches to the selected worktree.
func (v *WorktreeView) Enter(env *Env) tea.Cmd {
	r := v.current()
	if r == nil {
		return nil
	}
	switch {
	case r.wt.Prunable:
		return Errorf(fmt.Errorf("worktree %s is prunable (%s); cannot switch", env.Abbrev(r.wt.Path), orDefault(r.wt.PrunableReason, "directory missing")))
	case r.wt.Bare:
		return Errorf(fmt.Errorf("worktree %s is bare; cannot switch", env.Abbrev(r.wt.Path)))
	}
	if _, err := os.Stat(r.wt.Path); err != nil {
		return Errorf(fmt.Errorf("worktree %s: %w", env.Abbrev(r.wt.Path), err))
	}
	return Msg(SwitchWorktreeMsg{Path: r.wt.Path})
}

func orDefault(s, d string) string {
	if s == "" {
		return d
	}
	return s
}

func (v *WorktreeView) Do(env *Env, a keymap.Action) (tea.Cmd, bool) {
	r := v.current()
	gr := env.Git()
	switch a {
	case keymap.WorktreeAdd:
		return v.add(env), true
	case keymap.WorktreePrune:
		return func() tea.Msg {
			out, err := git.PruneWorktrees(context.Background(), gr, true)
			if err != nil {
				return StatusMsg{Text: err.Error(), Err: true}
			}
			preview := strings.TrimSpace(string(out))
			if preview == "" {
				return StatusMsg{Text: "Nothing to prune"}
			}
			q := "Prune: " + strings.ReplaceAll(preview, "\n", "; ") + "?"
			return ConfirmMsg{Question: q, OnYes: func() tea.Msg {
				if _, err := git.PruneWorktrees(context.Background(), gr, false); err != nil {
					return StatusMsg{Text: err.Error(), Err: true}
				}
				return tea.BatchMsg{Status("Pruned stale worktrees"), Msg(ReloadMsg{})}
			}}
		}, true
	}
	if r == nil {
		return nil, false
	}
	path := r.wt.Path
	switch a {
	case keymap.WorktreeRemove:
		if r.wt.Main {
			return Errorf(fmt.Errorf("the main worktree cannot be removed")), true
		}
		if v.isCurrent(env, r.wt) {
			return Errorf(fmt.Errorf("cannot remove the active worktree; switch to another one first")), true
		}
		dirty := r.dirty == 1
		remove := func(force bool) tea.Cmd {
			return func() tea.Msg {
				if _, err := git.RemoveWorktree(context.Background(), gr, path, force); err != nil {
					return tea.BatchMsg{Errorf(err), Msg(ReloadMsg{})}
				}
				return tea.BatchMsg{Status("Removed worktree " + env.Abbrev(path)), Msg(ReloadMsg{})}
			}
		}
		if dirty {
			// A dirty worktree needs a second, explicit confirmation for --force.
			return Confirm(env, a, "Worktree "+env.Abbrev(path)+" has uncommitted changes. Remove it?",
				Msg(ConfirmMsg{Question: "Really discard the changes and remove with --force?", OnYes: remove(true)})), true
		}
		return Confirm(env, a, "Remove worktree "+env.Abbrev(path)+"?", remove(false)), true
	case keymap.WorktreeLock:
		if r.wt.Locked {
			return RunGit(env, "Unlocked "+env.Abbrev(path), "worktree", "unlock", "--", path), true
		}
		return Msg(PromptMsg{Label: "Lock reason (optional): ", OnSubmit: func(reason string) tea.Cmd {
			return func() tea.Msg {
				if _, err := git.LockWorktree(context.Background(), gr, path, strings.TrimSpace(reason)); err != nil {
					return tea.BatchMsg{Errorf(err), Msg(ReloadMsg{})}
				}
				return tea.BatchMsg{Status("Locked " + env.Abbrev(path)), Msg(ReloadMsg{})}
			}
		}}), true
	case keymap.WorktreeOpen:
		if r.wt.Prunable || r.wt.Bare {
			return Errorf(fmt.Errorf("cannot open %s", env.Abbrev(path))), true
		}
		argv := make([]string, 0, len(env.Cfg.Worktree.OpenCommand))
		for _, a := range env.Cfg.Worktree.OpenCommand {
			argv = append(argv, os.ExpandEnv(a))
		}
		if argv[0] == "" {
			argv[0] = "sh"
		}
		cmd := exec.Command(argv[0], argv[1:]...)
		cmd.Dir = path
		return Msg(ExecMsg{Cmd: cmd}), true
	}
	return nil, false
}

func (v *WorktreeView) add(env *Env) tea.Cmd {
	gr := env.Git()
	base := filepath.Dir(env.WorktreeDir())
	return Msg(PromptMsg{Label: "New worktree path: ", Initial: base + string(filepath.Separator), OnSubmit: func(p string) tea.Cmd {
		p = strings.TrimSpace(p)
		if p == "" {
			return Status("Cancelled")
		}
		if strings.HasPrefix(p, "~/") && env.HomeDir != "" {
			p = filepath.Join(env.HomeDir, p[2:])
		}
		return Msg(PromptMsg{Label: "Branch (existing or new; empty = detached HEAD): ", Initial: filepath.Base(p), OnSubmit: func(b string) tea.Cmd {
			b = strings.TrimSpace(b)
			return func() tea.Msg {
				ctx := context.Background()
				newBranch := b != "" && !git.RefExists(ctx, gr, "refs/heads/"+b)
				if _, err := git.AddWorktree(ctx, gr, p, b, newBranch); err != nil {
					return tea.BatchMsg{Errorf(err), Msg(ReloadMsg{})}
				}
				msg := "Added worktree " + env.Abbrev(p)
				if newBranch {
					msg += " (new branch " + b + ")"
				}
				return tea.BatchMsg{Status(msg), Msg(ReloadMsg{})}
			}
		}})
	}})
}

func (v *WorktreeView) Selection(env *Env) Selection {
	if r := v.current(); r != nil {
		sel := Selection{Commit: r.wt.Head, Rev: r.wt.Head, Worktree: r.wt.Path}
		for _, ref := range env.Refs {
			if ref.Kind == git.RefBranch && ref.Short == r.wt.Branch {
				rf := ref
				sel.Ref = &rf
			}
		}
		return sel
	}
	return Selection{}
}
