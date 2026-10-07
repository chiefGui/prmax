package daemon

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"prmax/internal/config"
	"prmax/internal/model"
)

const (
	maxDescription = 4000
	maxDiff        = 150 << 10
)

type promptData struct {
	Title       string
	Description string
	Since       string
	Range       string
	Reported    []string
	Commits     string
	Stat        string
	Diff        string
	DiffSize    string
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
	from, diffRange, logRange := base, base+"...HEAD", base+"..HEAD"
	if j.review.SinceSHA != "" {
		from = j.review.SinceSHA
		diffRange, logRange = from+"..HEAD", from+"..HEAD"
	}
	git := func(args ...string) (string, error) {
		out, err := run(ctx, dir, nil, "", "git", args...)
		return strings.TrimRight(out, "\r\n"), err
	}
	data := promptData{Title: j.pr.Title, Description: desc, Since: j.review.SinceSHA, Range: "git diff " + diffRange}
	if data.Commits, err = git("log", "--no-merges", "--format=%h %s", logRange); err != nil {
		return "", err
	}
	if data.Stat, err = git("diff", "--stat", diffRange); err != nil {
		return "", err
	}
	diff, err := git("diff", diffRange)
	if err != nil {
		return "", err
	}
	if len(diff) <= maxDiff {
		data.Diff = diff
	} else {
		data.DiffSize = fmt.Sprintf("%d KB", len(diff)>>10)
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
