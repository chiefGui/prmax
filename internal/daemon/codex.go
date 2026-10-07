package daemon

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"prmax/internal/config"
	"prmax/internal/proc"
)

func resolveCodex(p string) string {
	if p == "" {
		p = "codex"
	}
	lp, err := exec.LookPath(p)
	if err != nil {
		return p
	}
	if !strings.HasSuffix(strings.ToLower(lp), ".cmd") {
		return lp
	}
	matches, _ := filepath.Glob(filepath.Join(filepath.Dir(lp), "node_modules", "@openai", "codex", "node_modules", "@openai", "codex-win32-*", "vendor", "*", "bin", "codex.exe"))
	if len(matches) > 0 {
		return matches[0]
	}
	return lp
}

func (d *Daemon) codex(ctx context.Context, j *job, dir, prompt string) (*reviewResult, error) {
	schema := filepath.Join(config.Dir(), "schema.json")
	if err := os.WriteFile(schema, []byte(reviewSchema), 0o644); err != nil {
		return nil, err
	}
	last := filepath.Join(config.Dir(), "logs", j.review.ID+".last.json")
	defer os.Remove(last)
	args := []string{
		"exec",
		"--json",
		"--skip-git-repo-check",
		"--sandbox", "read-only",
		"-c", `approval_policy="never"`,
		"--output-schema", schema,
		"-o", last,
		"-C", dir,
	}
	if j.review.Model != "" {
		args = append(args, "-m", j.review.Model)
	}
	if j.review.Effort != "" {
		args = append(args, "-c", fmt.Sprintf("model_reasoning_effort=%q", j.review.Effort))
	}
	args = append(args, "-")
	cmd := exec.Command(resolveCodex(d.cfg.CodexPath), args...)
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

	var failure string
	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 1<<20), 1<<26)
	for sc.Scan() {
		var ev map[string]any
		if json.Unmarshal(sc.Bytes(), &ev) != nil {
			continue
		}
		if f := d.renderCodexEvent(j, ev); f != "" {
			failure = f
		}
	}
	waitErr := tree.Wait()
	close(exited)
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	b, err := os.ReadFile(last)
	if err != nil || len(strings.TrimSpace(string(b))) == 0 {
		msg := failure
		if msg == "" {
			msg = strings.TrimSpace(stderr.String())
		}
		if msg == "" && waitErr != nil {
			msg = waitErr.Error()
		}
		return nil, fmt.Errorf("codex produced no result: %s", truncate(msg, 500))
	}
	var res reviewResult
	if err := json.Unmarshal([]byte(extractJSON(string(b))), &res); err != nil {
		return nil, fmt.Errorf("could not parse codex result: %s", truncate(string(b), 300))
	}
	return &res, nil
}

func (d *Daemon) renderCodexEvent(j *job, ev map[string]any) string {
	item, _ := ev["item"].(map[string]any)
	str := func(m map[string]any, k string) string { s, _ := m[k].(string); return s }
	switch ev["type"] {
	case "thread.started":
		d.logf(j, "info", "codex session started (%s)", j.review.Model)
	case "item.started":
		if str(item, "type") == "command_execution" {
			d.logf(j, "tool", "$ %s", truncate(str(item, "command"), 200))
		}
	case "item.completed":
		switch str(item, "type") {
		case "agent_message":
			t := strings.TrimSpace(str(item, "text"))
			if t == "" {
				return ""
			}
			if strings.HasPrefix(t, "{") {
				d.logf(j, "info", "writing result")
			} else {
				d.logf(j, "text", "%s", t)
			}
		case "reasoning":
			if t := strings.TrimSpace(str(item, "text")); t != "" {
				d.logf(j, "info", "%s", truncate(strings.SplitN(t, "\n", 2)[0], 200))
			}
		case "command_execution":
			if code, ok := item["exit_code"].(float64); ok && code != 0 {
				d.logf(j, "error", "exit %d: %s", int(code), truncate(str(item, "aggregated_output"), 300))
			}
		case "error":
			msg := str(item, "message")
			if strings.Contains(msg, "unrecognized configuration") {
				d.logf(j, "info", "%s", strings.SplitN(msg, "\n", 2)[0])
			} else {
				d.logf(j, "error", "%s", msg)
			}
		}
	case "turn.completed":
		usage, _ := ev["usage"].(map[string]any)
		in, _ := usage["input_tokens"].(float64)
		out, _ := usage["output_tokens"].(float64)
		d.logf(j, "info", "finished, %s in / %s out tokens", tokens(in), tokens(out))
	case "turn.failed":
		e, _ := ev["error"].(map[string]any)
		msg := str(e, "message")
		d.logf(j, "error", "%s", msg)
		return msg
	case "error":
		msg := str(ev, "message")
		d.logf(j, "error", "%s", msg)
		return msg
	}
	return ""
}

func tokens(n float64) string {
	if n >= 1000 {
		return fmt.Sprintf("%.1fk", n/1000)
	}
	return fmt.Sprintf("%d", int(n))
}
