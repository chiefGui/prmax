package daemon

import (
	"bufio"
	"context"
	"io"
	"log"
	"os/exec"
	"strings"
	"time"

	"prmax/internal/model"
	"prmax/internal/proc"
)

func (d *Daemon) forward(ctx context.Context, repo string) {
	d.deleteStaleHooks(ctx, repo)
	backoff := 2 * time.Second
	for ctx.Err() == nil {
		started := time.Now()
		err := d.forwardOnce(ctx, repo)
		if ctx.Err() != nil {
			return
		}
		msg := "forwarder exited"
		if err != nil {
			msg = err.Error()
		}
		log.Printf("forward %s: %s", repo, msg)
		d.store.SetForwarder(model.Forwarder{Repo: repo, Connected: false, Error: msg})
		if strings.Contains(strings.ToLower(msg), "already exists") {
			d.deleteStaleHooks(ctx, repo)
		}
		if time.Since(started) > time.Minute {
			backoff = 2 * time.Second
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		backoff = min(backoff*2, time.Minute)
	}
}

func (d *Daemon) forwardOnce(ctx context.Context, repo string) error {
	url := "http://" + d.cfg.Listen + "/webhook/" + repo
	cmd := exec.Command(d.cfg.GhPath, "webhook", "forward", "--repo", repo, "--events", "pull_request", "--url", url)
	pr, pw := io.Pipe()
	cmd.Stdout = pw
	cmd.Stderr = pw
	tree, err := proc.Start(cmd)
	if err != nil {
		return err
	}
	done := make(chan error, 1)
	go func() {
		err := tree.Wait()
		pw.Close()
		done <- err
	}()
	exited := make(chan struct{})
	defer close(exited)
	go func() {
		select {
		case <-ctx.Done():
			tree.Kill()
		case <-exited:
		}
	}()
	var errs []string
	connected := false
	sc := bufio.NewScanner(pr)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		switch {
		case line == "":
		case !connected && strings.Contains(strings.ToLower(line), "forwarding"):
			connected = true
			d.store.SetForwarder(model.Forwarder{Repo: repo, Connected: true})
			go d.catchUp(ctx, repo)
		case strings.HasPrefix(line, "Usage:"):
			for sc.Scan() {
			}
		default:
			errs = append(errs, line)
		}
	}
	err = <-done
	if len(errs) > 0 {
		return &forwardErr{msg: strings.Join(errs, " ")}
	}
	return err
}

type forwardErr struct{ msg string }

func (e *forwardErr) Error() string { return e.msg }

func (d *Daemon) deleteStaleHooks(ctx context.Context, repo string) {
	out, err := run(ctx, "", nil, "", d.cfg.GhPath, "api", "repos/"+repo+"/hooks", "--jq", `.[] | select(.config.url != null and (.config.url | test("webhook-forwarder|cli-webhook-forwarding"))) | .id`)
	if err != nil {
		log.Printf("forward %s: list hooks: %v", repo, err)
		return
	}
	for _, id := range strings.Fields(out) {
		if _, err := run(ctx, "", nil, "", d.cfg.GhPath, "api", "-X", "DELETE", "repos/"+repo+"/hooks/"+id); err != nil {
			log.Printf("forward %s: delete hook %s: %v", repo, id, err)
		} else {
			log.Printf("forward %s: deleted stale hook %s", repo, id)
		}
	}
}
