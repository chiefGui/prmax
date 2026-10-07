package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"prmax/internal/config"
	"prmax/internal/model"
)

type desktopSession struct {
	CLISessionID   string `json:"cliSessionId"`
	Title          string `json:"title"`
	IsArchived     bool   `json:"isArchived"`
	LastActivityAt int64  `json:"lastActivityAt"`
	PRs            []struct {
		Repo     string `json:"repo"`
		PRNumber int    `json:"prNumber"`
	} `json:"prs"`
}

type liveSession struct {
	SessionID string `json:"sessionId"`
	Name      string `json:"name"`
}

func desktopSessions(repo string, number int) []desktopSession {
	base, err := os.UserConfigDir()
	if err != nil {
		return nil
	}
	files, _ := filepath.Glob(filepath.Join(base, "Claude", "claude-code-sessions", "*", "*", "local_*.json"))
	var out []desktopSession
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		var s desktopSession
		if json.Unmarshal(b, &s) != nil || s.IsArchived || s.CLISessionID == "" {
			continue
		}
		for _, pr := range s.PRs {
			if strings.EqualFold(pr.Repo, repo) && pr.PRNumber == number {
				out = append(out, s)
				break
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].LastActivityAt > out[j].LastActivityAt })
	return out
}

func (d *Daemon) sessionFor(ctx context.Context, repo string, number int) (string, error) {
	linked := desktopSessions(repo, number)
	if len(linked) == 0 {
		return "", fmt.Errorf("no desktop session is linked to #%d", number)
	}
	out, err := run(ctx, "", nil, "", d.cfg.ClaudePath, "agents", "--json")
	if err != nil {
		return "", err
	}
	var live []liveSession
	if err := json.Unmarshal([]byte(out), &live); err != nil {
		return "", fmt.Errorf("claude agents: %w", err)
	}
	names := map[string]int{}
	for _, l := range live {
		names[l.Name]++
	}
	for _, s := range linked {
		for _, l := range live {
			if l.SessionID != s.CLISessionID {
				continue
			}
			if names[l.Name] > 1 {
				return "", fmt.Errorf("more than one running session is named %q", l.Name)
			}
			return l.Name, nil
		}
	}
	return "", fmt.Errorf("the session for #%d (%q) isn't running; open it in the desktop app", number, linked[0].Title)
}

func (d *Daemon) nudge(key string) {
	p, ok := d.store.Get(key)
	if !ok {
		return
	}
	r := lastCompleted(p)
	if r == nil {
		return
	}
	ctx, cancel := context.WithTimeout(d.ctx, 3*time.Minute)
	defer cancel()
	err := d.sendNudge(ctx, p, *r)
	d.store.Update(func(prs map[string]*model.PR) {
		cur, ok := prs[key]
		if !ok {
			return
		}
		for i := range cur.Reviews {
			if cur.Reviews[i].ID != r.ID {
				continue
			}
			if err != nil {
				cur.Reviews[i].NudgeError = err.Error()
			} else {
				cur.Reviews[i].NudgedAt = time.Now()
				cur.Reviews[i].NudgeError = ""
			}
		}
	})
}

func (d *Daemon) sendNudge(ctx context.Context, p model.PR, r model.Review) error {
	name, err := d.sessionFor(ctx, p.Repo, p.Number)
	if err != nil {
		return err
	}
	dir := filepath.Join(config.Root(), "nudges")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(dir, r.ID+".md")
	if err := os.WriteFile(path, []byte(renderNudge(p, r)), 0o644); err != nil {
		return err
	}
	line := fmt.Sprintf("prmax: round %d review for PR #%d is ready. Read %s and act on it.", r.Round, p.Number, path)
	instruction := fmt.Sprintf("Use SendMessage to send exactly this message to the session named %q:\n\n%s\n\nThen reply with only DONE, or the error.", name, line)
	out, err := run(ctx, "", nil, instruction, d.cfg.ClaudePath, "-p", "--model", "haiku", "--no-session-persistence", "--dangerously-skip-permissions")
	if err != nil {
		return err
	}
	if !strings.Contains(out, "DONE") {
		return fmt.Errorf("nudge not delivered: %s", truncate(out, 200))
	}
	return nil
}

func renderNudge(p model.PR, r model.Review) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Review round %d on PR #%d", r.Round, p.Number)
	if r.CommentURL != "" {
		fmt.Fprintf(&b, ": %s", r.CommentURL)
	}
	b.WriteString("\n")
	if len(r.Findings) == 0 {
		b.WriteString("\nNo findings.\n")
	}
	for _, c := range model.Categories {
		fs := r.ByCategory(c.ID)
		if len(fs) == 0 {
			continue
		}
		fmt.Fprintf(&b, "\n%s\n", c.Label)
		for i, f := range fs {
			title := strings.TrimSpace(f.Title)
			if f.StillOpen {
				title += " (still open)"
			}
			fmt.Fprintf(&b, "%d. %s · %s\n   %s\n", i+1, title, location(f), strings.TrimSpace(f.Text))
		}
	}
	b.WriteString("\n---\n\n- Address what you're confident in.\n- Skip what doesn't make sense.\n- Reply on the PR concisely: what you addressed, what you skipped and why.\n")
	return b.String()
}
