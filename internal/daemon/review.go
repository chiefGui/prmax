package daemon

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os/exec"
	"strings"
	"time"

	"prmax/internal/config"
	"prmax/internal/model"
	"prmax/internal/proc"
)

const reviewSchema = `{
  "type": "object",
  "additionalProperties": false,
  "required": ["verdict", "summary", "findings", "previous"],
  "properties": {
    "verdict": {"type": "string", "enum": ["clean", "findings"]},
    "summary": {"type": "string"},
    "findings": {
      "type": "array",
      "items": {
        "type": "object",
        "additionalProperties": false,
        "required": ["severity", "file", "line", "title", "detail"],
        "properties": {
          "severity": {"type": "string", "enum": ["high", "medium", "low"]},
          "file": {"type": "string"},
          "line": {"type": "integer"},
          "title": {"type": "string"},
          "detail": {"type": "string"}
        }
      }
    },
    "previous": {
      "type": "array",
      "items": {
        "type": "object",
        "additionalProperties": false,
        "required": ["title", "status", "note"],
        "properties": {
          "title": {"type": "string"},
          "status": {"type": "string", "enum": ["fixed", "open", "obsolete"]},
          "note": {"type": "string"}
        }
      }
    }
  }
}`

var allowedTools = []string{
	"Read", "Grep", "Glob",
	"Bash(git diff:*)", "Bash(git log:*)", "Bash(git show:*)",
	"Bash(git blame:*)", "Bash(git merge-base:*)", "Bash(git status:*)",
	"Bash(git ls-files:*)", "Bash(git grep:*)",
}

type reviewResult struct {
	Verdict  string           `json:"verdict"`
	Summary  string           `json:"summary"`
	Findings []model.Finding  `json:"findings"`
	Previous []model.Previous `json:"previous"`
}

type job struct {
	key       string
	pr        model.PR
	repo      config.Repo
	review    model.Review
	openItems []string
}

func (d *Daemon) runReview(ctx context.Context, key string, req model.ReviewRequest) {
	p, ok := d.store.Get(key)
	if !ok {
		return
	}
	repo, ok := d.cfg.Repo(p.Repo)
	if !ok {
		return
	}
	j := &job{key: key, pr: p, repo: repo}
	j.review = model.Review{
		ID:         reviewID(p),
		Repo:       p.Repo,
		Number:     p.Number,
		Kind:       model.KindFull,
		Provider:   req.Provider,
		Model:      req.Model,
		Effort:     req.Effort,
		ModelLabel: d.label(req.Provider, req.Model, req.Effort),
		HeadSHA:    p.HeadSHA,
		Status:     model.StatusReviewing,
		StartedAt:  time.Now(),
	}
	d.store.Update(func(prs map[string]*model.PR) {
		if cur, ok := prs[key]; ok {
			cur.Status = model.StatusReviewing
			cur.Reviews = append(cur.Reviews, j.review)
			trimReviews(cur)
		}
	})
	d.logf(j, "info", "review %s at %s with %s", p.Key(), short(p.HeadSHA), j.review.ModelLabel)

	err := d.execute(ctx, j)
	j.review.FinishedAt = time.Now()
	switch {
	case d.ctx.Err() != nil:
		j.review.Status = model.StatusFailed
		j.review.Error = "interrupted"
	case errors.Is(ctx.Err(), context.Canceled):
		j.review.Status = model.StatusFailed
		j.review.Error = "canceled"
		d.logf(j, "info", "canceled")
	case err != nil:
		j.review.Status = model.StatusFailed
		j.review.Error = err.Error()
		d.logf(j, "error", "%v", err)
	}
	d.store.CloseLog(j.review.ID)

	d.store.Update(func(prs map[string]*model.PR) {
		cur, ok := prs[key]
		if !ok {
			return
		}
		for i := range cur.Reviews {
			if cur.Reviews[i].ID == j.review.ID {
				cur.Reviews[i] = j.review
			}
		}
		if j.review.Status == model.StatusClean || j.review.Status == model.StatusFindings {
			cur.ReviewedSHA = j.review.HeadSHA
		}
		cur.Status = model.StatusIdle
		settle(cur)
	})
}

func (d *Daemon) execute(ctx context.Context, j *job) error {
	p := j.pr
	lock := d.repoLock(p.Repo)
	lock.Lock()
	base := p.BaseBranch
	if base == "" {
		base = "main"
	}
	_, err := run(ctx, j.repo.Path, nil, "", "git", "fetch", "--no-tags", "origin",
		fmt.Sprintf("+refs/pull/%d/head:refs/prmax/pr/%d", p.Number, p.Number),
		fmt.Sprintf("+refs/heads/%s:refs/prmax/base/%s", base, base))
	if err != nil {
		lock.Unlock()
		return err
	}
	lock.Unlock()
	d.logf(j, "info", "checking out %s", short(p.HeadSHA))
	wt, release, err := d.checkout(ctx, j.repo, p.HeadSHA)
	if err != nil {
		return err
	}
	defer release()

	mb, err := run(ctx, wt, nil, "", "git", "merge-base", "refs/prmax/base/"+base, "HEAD")
	if err != nil {
		return err
	}
	baseSHA := strings.TrimSpace(mb)

	prev := lastCompleted(p)
	if prev != nil && p.ReviewedSHA != "" && p.ReviewedSHA != p.HeadSHA {
		if _, err := run(ctx, wt, nil, "", "git", "merge-base", "--is-ancestor", p.ReviewedSHA, "HEAD"); err == nil {
			j.review.Kind = model.KindIncremental
			j.review.SinceSHA = p.ReviewedSHA
			j.openItems = openItems(prev)
		} else {
			d.logf(j, "info", "previous review commit %s is gone (force push?), doing a full review", short(p.ReviewedSHA))
		}
	}
	d.syncReview(j)

	prompt := d.prompt(j, baseSHA)
	var res *reviewResult
	if j.review.Provider == "codex" {
		res, err = d.codex(ctx, j, wt, prompt)
	} else {
		res, err = d.claude(ctx, j, wt, prompt)
	}
	if err != nil {
		return err
	}
	j.review.Summary = res.Summary
	j.review.Findings = res.Findings
	j.review.Previous = res.Previous
	if len(res.Findings) == 0 && !anyOpen(res.Previous) {
		j.review.Status = model.StatusClean
	} else {
		j.review.Status = model.StatusFindings
	}

	url, err := d.postComment(ctx, p.Repo, p.Number, renderComment(j.review))
	if err != nil {
		d.logf(j, "error", "post comment: %v", err)
		j.review.Error = "comment not posted: " + err.Error()
		return nil
	}
	j.review.CommentURL = url
	d.logf(j, "done", "posted %s", url)
	return nil
}

func (d *Daemon) syncReview(j *job) {
	d.store.Update(func(prs map[string]*model.PR) {
		if cur, ok := prs[j.key]; ok {
			for i := range cur.Reviews {
				if cur.Reviews[i].ID == j.review.ID {
					cur.Reviews[i] = j.review
				}
			}
		}
	})
}

func (d *Daemon) prompt(j *job, baseSHA string) string {
	p := j.pr
	var b strings.Builder
	fmt.Fprintf(&b, "You are reviewing pull request #%d in %s.\n\n", p.Number, p.Repo)
	fmt.Fprintf(&b, "Title: %s\nAuthor: %s\nBranch: %s -> %s\n", p.Title, p.Author, p.Branch, p.BaseBranch)
	if body := strings.TrimSpace(p.Body); body != "" {
		if len(body) > 4000 {
			body = body[:4000] + "\n[truncated]"
		}
		fmt.Fprintf(&b, "\nDescription:\n%s\n", body)
	}
	fmt.Fprintf(&b, "\nThe PR head (%s) is checked out in the current directory as a detached worktree. Large binary files may be Git LFS pointers; ignore them.\n", p.HeadSHA)
	if j.review.Kind == model.KindIncremental {
		fmt.Fprintf(&b, "\nThis is a re-review. The PR was last reviewed at %s. Focus on what changed since then: `git log %s..HEAD` and `git diff %s..HEAD`. Use `git diff %s...HEAD` only when you need the wider context of the whole PR.\n", j.review.SinceSHA, j.review.SinceSHA, j.review.SinceSHA, baseSHA)
		if len(j.openItems) > 0 {
			b.WriteString("\nFindings still open from the previous review. For each one, report in `previous` whether it is now fixed, still open, or obsolete:\n")
			for _, it := range j.openItems {
				fmt.Fprintf(&b, "- %s\n", it)
			}
		}
		b.WriteString("\nReport only new problems in `findings`; do not repeat previous findings there.\n")
	} else {
		fmt.Fprintf(&b, "\nThe PR's changes are `git diff %s...HEAD` (merge base %s). Use `git log %s..HEAD` for the commits.\n", baseSHA, baseSHA, baseSHA)
		b.WriteString("Leave `previous` empty.\n")
	}
	b.WriteString(`
Review for correctness bugs, regressions, crashes, data loss, security problems, race conditions and broken edge cases. Read the surrounding code to confirm each issue is real. Ignore style, naming, formatting and matters of taste. Only report problems you are confident about; an empty findings list is a good outcome when the code is correct.

If the repository has AGENTS.md or CLAUDE.md, follow its conventions when judging the change.

Do not modify any files. For each finding give the file path relative to the repo root, the most relevant line in the PR head, a short title, and a detail that explains the failure scenario concretely. The summary is one or two sentences about the PR and the overall result.
`)
	if s := strings.TrimSpace(d.cfg.Instructions); s != "" {
		fmt.Fprintf(&b, "\nAdditional instructions:\n%s\n", s)
	}
	return b.String()
}

func (d *Daemon) claude(ctx context.Context, j *job, dir, prompt string) (*reviewResult, error) {
	args := []string{
		"-p",
		"--output-format", "stream-json",
		"--verbose",
		"--no-session-persistence",
		"--json-schema", reviewSchema,
		"--tools", "Read,Grep,Glob,Bash",
		"--allowedTools", strings.Join(allowedTools, ","),
	}
	if j.review.Model != "" {
		args = append(args, "--model", j.review.Model)
	}
	if j.review.Effort != "" {
		args = append(args, "--effort", j.review.Effort)
	}
	cmd := exec.Command(d.cfg.ClaudePath, args...)
	cmd.Dir = dir
	cmd.Stdin = strings.NewReader(prompt)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	var stderr strings.Builder
	cmd.Stderr = &limitedWriter{w: &stderr, n: 8192}
	tree, err := proc.Start(cmd)
	if err != nil {
		return nil, err
	}
	exited := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			tree.Kill()
		case <-exited:
		}
	}()

	var final map[string]any
	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 1<<20), 1<<26)
	for sc.Scan() {
		var ev map[string]any
		if json.Unmarshal(sc.Bytes(), &ev) != nil {
			continue
		}
		if ev["type"] == "result" {
			final = ev
		}
		d.renderEvent(j, ev)
	}
	waitErr := tree.Wait()
	close(exited)
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if final == nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" && waitErr != nil {
			msg = waitErr.Error()
		}
		return nil, fmt.Errorf("claude produced no result: %s", msg)
	}
	if c, ok := final["total_cost_usd"].(float64); ok {
		j.review.CostUSD = c
	}
	if isErr, _ := final["is_error"].(bool); isErr {
		r, _ := final["result"].(string)
		return nil, fmt.Errorf("claude: %s", r)
	}
	var res reviewResult
	if so, ok := final["structured_output"]; ok && so != nil {
		b, _ := json.Marshal(so)
		if err := json.Unmarshal(b, &res); err != nil {
			return nil, fmt.Errorf("structured output: %w", err)
		}
		return &res, nil
	}
	r, _ := final["result"].(string)
	if err := json.Unmarshal([]byte(extractJSON(r)), &res); err != nil {
		return nil, fmt.Errorf("could not parse review result: %s", truncate(r, 300))
	}
	return &res, nil
}

func (d *Daemon) renderEvent(j *job, ev map[string]any) {
	switch ev["type"] {
	case "system":
		if ev["subtype"] == "init" {
			m, _ := ev["model"].(string)
			d.logf(j, "info", "claude session started (%s)", m)
		}
	case "assistant":
		for _, c := range contentItems(ev) {
			switch c["type"] {
			case "text":
				if t, _ := c["text"].(string); strings.TrimSpace(t) != "" {
					d.logf(j, "text", "%s", strings.TrimSpace(t))
				}
			case "tool_use":
				name, _ := c["name"].(string)
				if name == "StructuredOutput" {
					d.logf(j, "info", "writing result")
					continue
				}
				in, _ := c["input"].(map[string]any)
				d.logf(j, "tool", "%s %s", name, toolSummary(name, in))
			}
		}
	case "user":
		for _, c := range contentItems(ev) {
			if c["type"] == "tool_result" {
				if isErr, _ := c["is_error"].(bool); isErr {
					d.logf(j, "error", "%s", truncate(toolResultText(c), 300))
				}
			}
		}
	case "result":
		c, _ := ev["total_cost_usd"].(float64)
		turns, _ := ev["num_turns"].(float64)
		dur, _ := ev["duration_ms"].(float64)
		d.logf(j, "info", "finished in %s, %d turns, $%.2f", (time.Duration(dur) * time.Millisecond).Round(time.Second), int(turns), c)
	}
}

func (d *Daemon) logf(j *job, kind, format string, args ...any) {
	l := model.LogLine{ReviewID: j.review.ID, Time: time.Now(), Kind: kind, Text: fmt.Sprintf(format, args...)}
	if kind == "error" {
		log.Printf("%s: %s", j.key, l.Text)
	}
	d.store.AppendLog(l)
}

func contentItems(ev map[string]any) []map[string]any {
	msg, _ := ev["message"].(map[string]any)
	raw, _ := msg["content"].([]any)
	var out []map[string]any
	for _, r := range raw {
		if m, ok := r.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

func toolSummary(name string, in map[string]any) string {
	str := func(k string) string { s, _ := in[k].(string); return s }
	switch name {
	case "Read":
		return str("file_path")
	case "Grep":
		s := str("pattern")
		if p := str("path"); p != "" {
			s += "  in " + p
		}
		if g := str("glob"); g != "" {
			s += "  (" + g + ")"
		}
		return s
	case "Glob":
		return str("pattern")
	case "Bash":
		return truncate(str("command"), 200)
	}
	b, _ := json.Marshal(in)
	return truncate(string(b), 200)
}

func toolResultText(c map[string]any) string {
	switch v := c["content"].(type) {
	case string:
		return v
	case []any:
		var parts []string
		for _, x := range v {
			if m, ok := x.(map[string]any); ok {
				if t, ok := m["text"].(string); ok {
					parts = append(parts, t)
				}
			}
		}
		return strings.Join(parts, " ")
	}
	return ""
}

func lastCompleted(p model.PR) *model.Review {
	for i := len(p.Reviews) - 1; i >= 0; i-- {
		s := p.Reviews[i].Status
		if s == model.StatusClean || s == model.StatusFindings {
			return &p.Reviews[i]
		}
	}
	return nil
}

func openItems(r *model.Review) []string {
	var out []string
	for _, pv := range r.Previous {
		if pv.Status == "open" {
			out = append(out, pv.Title)
		}
	}
	for _, f := range r.Findings {
		loc := f.File
		if f.Line > 0 {
			loc = fmt.Sprintf("%s:%d", f.File, f.Line)
		}
		out = append(out, fmt.Sprintf("[%s] %s (%s): %s", f.Severity, f.Title, loc, truncate(f.Detail, 400)))
	}
	return out
}

func anyOpen(prev []model.Previous) bool {
	for _, p := range prev {
		if p.Status == "open" {
			return true
		}
	}
	return false
}

func reviewID(p model.PR) string {
	name := p.Repo
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
	}
	return fmt.Sprintf("%s-%s-%d-%s", time.Now().Format("20060102-150405"), strings.ToLower(name), p.Number, short(p.HeadSHA))
}

func short(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func extractJSON(s string) string {
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start < 0 || end < start {
		return s
	}
	return s[start : end+1]
}

type limitedWriter struct {
	w io.Writer
	n int
}

func (l *limitedWriter) Write(p []byte) (int, error) {
	if l.n <= 0 {
		return len(p), nil
	}
	k := min(len(p), l.n)
	l.n -= k
	l.w.Write(p[:k])
	return len(p), nil
}
