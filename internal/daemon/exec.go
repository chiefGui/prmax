package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"prmax/internal/model"
	"prmax/internal/proc"
)

func run(ctx context.Context, dir string, env []string, stdin string, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	proc.Hide(cmd)
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(errb.String())
		if msg == "" {
			msg = strings.TrimSpace(out.String())
		}
		return out.String(), fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, msg)
	}
	return out.String(), nil
}

type ghPR struct {
	Number      int       `json:"number"`
	Title       string    `json:"title"`
	Body        string    `json:"body"`
	URL         string    `json:"url"`
	HeadRefName string    `json:"headRefName"`
	HeadRefOid  string    `json:"headRefOid"`
	BaseRefName string    `json:"baseRefName"`
	IsDraft     bool      `json:"isDraft"`
	State       string    `json:"state"`
	UpdatedAt   time.Time `json:"updatedAt"`
	Author      struct {
		Login string `json:"login"`
	} `json:"author"`
}

const ghPRFields = "number,title,body,url,headRefName,headRefOid,baseRefName,isDraft,state,updatedAt,author"

func (g ghPR) apply(p *model.PR, repo string) {
	p.Repo = repo
	p.Number = g.Number
	p.Title = g.Title
	p.Body = g.Body
	p.URL = g.URL
	p.Branch = g.HeadRefName
	p.BaseBranch = g.BaseRefName
	setHead(p, g.HeadRefOid)
	p.Draft = g.IsDraft
	p.Author = g.Author.Login
	if g.UpdatedAt.After(p.UpdatedAt) {
		p.UpdatedAt = g.UpdatedAt
	}
}

func (d *Daemon) listOpenPRs(ctx context.Context, repo string) ([]ghPR, error) {
	out, err := run(ctx, "", nil, "", d.cfg.GhPath, "pr", "list", "-R", repo, "--state", "open", "--limit", "200", "--json", ghPRFields)
	if err != nil {
		return nil, err
	}
	var prs []ghPR
	return prs, json.Unmarshal([]byte(out), &prs)
}

func (d *Daemon) viewPR(ctx context.Context, repo string, number int) (ghPR, error) {
	out, err := run(ctx, "", nil, "", d.cfg.GhPath, "pr", "view", fmt.Sprint(number), "-R", repo, "--json", ghPRFields)
	var p ghPR
	if err != nil {
		return p, err
	}
	return p, json.Unmarshal([]byte(out), &p)
}

func (d *Daemon) postComment(ctx context.Context, repo string, number int, body string) (string, error) {
	env, err := d.postEnv(repo)
	if err != nil {
		return "", err
	}
	out, err := run(ctx, "", env, body, d.cfg.GhPath, "pr", "comment", fmt.Sprint(number), "-R", repo, "--body-file", "-")
	return strings.TrimSpace(out), err
}

func setHead(p *model.PR, sha string) {
	if p.HeadSHA != "" && p.HeadSHA != sha {
		p.Activity++
	}
	p.HeadSHA = sha
}
