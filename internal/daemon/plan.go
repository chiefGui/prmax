package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"prmax/internal/model"
)

func (d *Daemon) plan(ctx context.Context, key string) (model.Plan, error) {
	p, ok := d.store.Get(key)
	if !ok {
		return model.Plan{}, fmt.Errorf("unknown pr")
	}
	if g, err := d.viewPR(ctx, p.Repo, p.Number); err == nil {
		d.store.Update(func(prs map[string]*model.PR) {
			if cur, ok := prs[key]; ok {
				g.apply(cur, p.Repo)
				settle(cur)
			}
		})
		p, _ = d.store.Get(key)
	}
	pl := model.Plan{Round: rounds(p) + 1}
	prev := lastCompleted(p)
	if prev == nil || p.ReviewedSHA == "" || p.ReviewedSHA == p.HeadSHA {
		return pl, nil
	}
	out, err := run(ctx, "", nil, "", d.cfg.GhPath, "api", fmt.Sprintf("repos/%s/compare/%s...%s", p.Repo, p.ReviewedSHA, p.HeadSHA))
	if err != nil {
		return pl, err
	}
	var cmp struct {
		Status  string `json:"status"`
		Commits []struct {
			SHA    string `json:"sha"`
			Commit struct {
				Message string `json:"message"`
			} `json:"commit"`
		} `json:"commits"`
	}
	if err := json.Unmarshal([]byte(out), &cmp); err != nil {
		return pl, err
	}
	if cmp.Status != "ahead" {
		pl.Rewritten = true
		return pl, nil
	}
	pl.Since = p.ReviewedSHA
	pl.SinceRound = prev.Round
	pl.SinceIssues = len(prev.Findings)
	for _, c := range cmp.Commits {
		pl.Commits = append(pl.Commits, model.Commit{SHA: c.SHA, Title: strings.SplitN(c.Commit.Message, "\n", 2)[0]})
	}
	return pl, nil
}
