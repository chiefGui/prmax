package daemon

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"prmax/internal/config"
)

var lfsSkip = []string{"GIT_LFS_SKIP_SMUDGE=1"}

func (d *Daemon) checkout(ctx context.Context, repo config.Repo, sha string) (string, func(), error) {
	slot := d.acquireSlot(repo.Name)
	release := func() { d.releaseSlot(repo.Name, slot) }
	dir := filepath.Join(config.Dir(), "worktrees", fmt.Sprintf("%s-%d", strings.ToLower(filepath.Base(repo.Name)), slot))

	if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
		_, err := run(ctx, dir, lfsSkip, "", "git", "checkout", "--detach", "--force", sha)
		if err == nil {
			_, err = run(ctx, dir, nil, "", "git", "clean", "-fdq")
		}
		if err == nil {
			return dir, release, nil
		}
		if ctx.Err() != nil {
			release()
			return "", nil, err
		}
		run(ctx, repo.Path, nil, "", "git", "worktree", "remove", "--force", dir)
	}
	lock := d.repoLock(repo.Name)
	lock.Lock()
	defer lock.Unlock()
	os.RemoveAll(dir)
	run(ctx, repo.Path, nil, "", "git", "worktree", "prune")
	if _, err := run(ctx, repo.Path, lfsSkip, "", "git", "worktree", "add", "--detach", dir, sha); err != nil {
		release()
		return "", nil, err
	}
	return dir, release, nil
}

func (d *Daemon) acquireSlot(repo string) int {
	d.slotMu.Lock()
	defer d.slotMu.Unlock()
	for {
		busy := d.slots[repo]
		for i := 0; i < d.cfg.Concurrency; i++ {
			if !busy[i] {
				if busy == nil {
					busy = map[int]bool{}
					d.slots[repo] = busy
				}
				busy[i] = true
				return i
			}
		}
		d.slotCond.Wait()
	}
}

func (d *Daemon) releaseSlot(repo string, slot int) {
	d.slotMu.Lock()
	delete(d.slots[repo], slot)
	d.slotMu.Unlock()
	d.slotCond.Broadcast()
}
