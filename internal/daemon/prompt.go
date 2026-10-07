package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"prmax/internal/config"
	"prmax/internal/model"
)

const (
	maxDescription = 4000
	maxDiff        = 150 << 10
)

type promptData struct {
	Title          string
	Description    string
	Since          string
	Whole          bool
	Range          string
	Reported       []string
	Discussion     []string
	Commits        string
	ChangedCommits string
	ChangedStat    string
	Stat           string
	Diff           string
	DiffSize       string
}

func promptPath() string { return filepath.Join(config.Root(), "prompt.md") }

func promptTemplate() (*template.Template, error) {
	src := defaultPrompt
	if b, err := os.ReadFile(promptPath()); err == nil {
		src = string(b)
	}
	return template.New("prompt").Funcs(template.FuncMap{"inc": func(i int) int { return i + 1 }}).Parse(src)
}

func (d *Daemon) prompt(ctx context.Context, j *job, dir, base string) (string, error) {
	t, err := promptTemplate()
	if err != nil {
		return "", fmt.Errorf("%s: %w", promptPath(), err)
	}
	desc := strings.TrimSpace(j.pr.Body)
	if len(desc) > maxDescription {
		desc = desc[:maxDescription]
	}
	diffRange, logRange := base+"...HEAD", base+"..HEAD"
	git := func(args ...string) (string, error) {
		out, err := run(ctx, dir, nil, "", "git", args...)
		return strings.TrimRight(out, "\r\n"), err
	}
	data := promptData{Title: j.pr.Title, Description: desc, Since: j.review.SinceSHA, Whole: j.review.Scope == model.ScopeWhole, Range: "git diff " + diffRange}
	if data.Commits, err = git("log", "--no-merges", "--format=%h %s", logRange); err != nil {
		return "", err
	}
	if since := j.review.SinceSHA; since != "" {
		if data.ChangedCommits, err = git("log", "--no-merges", "--format=%h %s", since+"..HEAD"); err != nil {
			return "", err
		}
		if data.ChangedStat, err = git("diff", "--stat", since+"..HEAD"); err != nil {
			return "", err
		}
	}
	if data.Stat, err = git("diff", "--stat", diffRange); err != nil {
		return "", err
	}
	shown := diffRange
	if data.Since != "" && !data.Whole {
		shown = data.Since + "..HEAD"
	}
	diff, err := git("diff", shown)
	if err != nil {
		return "", err
	}
	if len(diff) <= maxDiff {
		data.Diff = diff
	} else {
		data.DiffSize = fmt.Sprintf("%d KB", len(diff)>>10)
	}
	if !j.since.IsZero() {
		data.Discussion = d.discussion(ctx, j.pr, j.since)
	}
	for _, f := range j.reported {
		data.Reported = append(data.Reported, fmt.Sprintf("`%s` %s: %s", location(f), f.Title, f.Text))
	}
	var b strings.Builder
	if err := t.Execute(&b, data); err != nil {
		return "", err
	}
	d.logf(j, "info", "prompt %d KB, diff %s", b.Len()>>10, diffNote(data))
	return b.String(), nil
}

func diffNote(p promptData) string {
	if p.Diff == "" {
		return "omitted (" + p.DiffSize + ")"
	}
	return fmt.Sprintf("included (%d KB)", len(p.Diff)>>10)
}

func location(f model.Finding) string {
	if f.Line > 0 {
		return fmt.Sprintf("%s:%d", f.File, f.Line)
	}
	return f.File
}

func rounds(p model.PR) int {
	n := 0
	for _, r := range p.Reviews {
		if r.Status == model.StatusClean || r.Status == model.StatusFindings {
			n++
		}
	}
	return n
}

const (
	maxComment    = 2000
	maxDiscussion = 8000
)

func (d *Daemon) discussion(ctx context.Context, p model.PR, since time.Time) []string {
	out, err := run(ctx, "", nil, "", d.cfg.GhPath, "api", "--paginate", fmt.Sprintf("repos/%s/issues/%d/comments", p.Repo, p.Number),
		"--jq", ".[] | {user: .user.login, at: .created_at, body: .body}")
	if err != nil {
		return nil
	}
	var lines []string
	total := 0
	dec := json.NewDecoder(strings.NewReader(out))
	for dec.More() {
		var c struct {
			User string    `json:"user"`
			At   time.Time `json:"at"`
			Body string    `json:"body"`
		}
		if dec.Decode(&c) != nil {
			break
		}
		body := strings.TrimSpace(c.Body)
		if c.At.Before(since) || body == "" || strings.Contains(body, "<!-- prmax:") {
			continue
		}
		if len(body) > maxComment {
			body = body[:maxComment]
		}
		entry := fmt.Sprintf("%s · %s\n%s", c.User, c.At.Local().Format("Jan 2 15:04"), body)
		if total+len(entry) > maxDiscussion {
			break
		}
		total += len(entry)
		lines = append(lines, entry)
	}
	return lines
}
