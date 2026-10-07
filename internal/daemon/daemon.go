package daemon

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"prmax/internal/config"
	"prmax/internal/model"
)

type running struct {
	sha    string
	cancel context.CancelFunc
}

type Daemon struct {
	cfg   config.Config
	store *Store

	mu      sync.Mutex
	pending map[string]model.ReviewRequest
	running map[string]*running
	queue   chan string

	locksMu sync.Mutex
	locks   map[string]*sync.Mutex

	slotMu   sync.Mutex
	slotCond *sync.Cond
	slots    map[string]map[int]bool

	ctx context.Context
	app *appAuth
}

func Run(cfg config.Config) error {
	if len(cfg.Repos) == 0 {
		return fmt.Errorf("no repos in %s", config.Path())
	}
	ln, err := net.Listen("tcp", cfg.Listen)
	if err != nil {
		return fmt.Errorf("listen %s (is the daemon already running?): %w", cfg.Listen, err)
	}
	dir := config.Dir()
	os.MkdirAll(dir, 0o755)
	lf, err := os.OpenFile(filepath.Join(dir, "daemon.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err == nil {
		log.SetOutput(lf)
	}
	store, err := NewStore(dir)
	if err != nil {
		return err
	}
	d := &Daemon{
		cfg:     cfg,
		store:   store,
		pending: map[string]model.ReviewRequest{},
		running: map[string]*running{},
		queue:   make(chan string, 4096),
		locks:   map[string]*sync.Mutex{},
		slots:   map[string]map[int]bool{},
	}
	d.slotCond = sync.NewCond(&d.slotMu)
	if cfg.AppID != "" {
		if d.app, err = newAppAuth(cfg.AppID, cfg.AppKeyPath()); err != nil {
			return fmt.Errorf("github app: %w", err)
		}
	}
	for _, r := range cfg.Repos {
		run(context.Background(), r.Path, nil, "", "git", "worktree", "prune")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	d.ctx = ctx
	for i := 0; i < cfg.Concurrency; i++ {
		go d.worker(ctx)
	}
	for _, r := range cfg.Repos {
		d.store.SetForwarder(model.Forwarder{Repo: r.Name})
		go d.forward(ctx, r.Name)
		go d.catchUp(ctx, r.Name)
	}
	log.Printf("prmax daemon listening on %s, repos: %d", cfg.Listen, len(cfg.Repos))
	srv := &http.Server{Handler: d.routes(cancel), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		srv.Close()
	}()
	err = srv.Serve(ln)
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}

func (d *Daemon) repoLock(repo string) *sync.Mutex {
	d.locksMu.Lock()
	defer d.locksMu.Unlock()
	l, ok := d.locks[repo]
	if !ok {
		l = &sync.Mutex{}
		d.locks[repo] = l
	}
	return l
}

func (d *Daemon) request(key string, req model.ReviewRequest) error {
	if req.Provider != "claude" && req.Provider != "codex" {
		return fmt.Errorf("unknown provider %q", req.Provider)
	}
	if req.Access == "" {
		req.Access = model.AccessFull
	}
	if req.Access != model.AccessFull && req.Access != model.AccessReadOnly {
		return fmt.Errorf("unknown access %q", req.Access)
	}
	if req.Scope == "" {
		req.Scope = model.ScopeChanges
	}
	if req.Scope != model.ScopeChanges && req.Scope != model.ScopeWhole {
		return fmt.Errorf("unknown scope %q", req.Scope)
	}
	p, ok := d.store.Get(key)
	if !ok {
		return fmt.Errorf("unknown pr")
	}
	if prev := lastCompleted(p); req.Scope == model.ScopeChanges && prev != nil && p.ReviewedSHA == p.HeadSHA {
		return fmt.Errorf("nothing changed since round %d", prev.Round)
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if _, ok := d.running[key]; ok {
		return fmt.Errorf("already reviewing")
	}
	if _, ok := d.pending[key]; ok {
		return fmt.Errorf("already queued")
	}
	d.pending[key] = req
	go saveChoice(req)
	d.setStatus(key, model.StatusQueued)
	d.queue <- key
	return nil
}

func (d *Daemon) setStatus(key, status string) {
	d.store.Update(func(prs map[string]*model.PR) {
		if p, ok := prs[key]; ok {
			p.Status = status
		}
	})
}

func (d *Daemon) worker(ctx context.Context) {
	for {
		var key string
		select {
		case <-ctx.Done():
			return
		case key = <-d.queue:
		}
		p, ok := d.store.Get(key)
		d.mu.Lock()
		req := d.pending[key]
		delete(d.pending, key)
		if !ok {
			d.mu.Unlock()
			continue
		}
		rctx, cancel := context.WithCancel(ctx)
		d.running[key] = &running{sha: p.HeadSHA, cancel: cancel}
		d.mu.Unlock()

		d.runReview(rctx, key, req)
		cancel()

		d.mu.Lock()
		delete(d.running, key)
		d.mu.Unlock()
	}
}

func (d *Daemon) cancel(key string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if r, ok := d.running[key]; ok {
		r.cancel()
	}
}

func (d *Daemon) catchUp(ctx context.Context, repo string) {
	prs, err := d.listOpenPRs(ctx, repo)
	if err != nil {
		log.Printf("catch up %s: %v", repo, err)
		return
	}
	open := map[string]bool{}
	for _, g := range prs {
		open[model.Key(repo, g.Number)] = true
	}
	var removed []string
	d.store.Update(func(m map[string]*model.PR) {
		for _, g := range prs {
			key := model.Key(repo, g.Number)
			p, ok := m[key]
			if !ok {
				p = &model.PR{Status: model.StatusIdle, Activity: 1}
				m[key] = p
			}
			g.apply(p, repo)
			settle(p)
		}
		for key, p := range m {
			if p.Repo == repo && !open[key] {
				delete(m, key)
				removed = append(removed, key)
			}
		}
	})
	for _, key := range removed {
		d.cancel(key)
	}
	log.Printf("catch up %s: %d open PRs", repo, len(prs))
}

func settle(p *model.PR) {
	switch {
	case p.Status == model.StatusQueued || p.Status == model.StatusReviewing:
	case failedAt(*p):
		p.Status = model.StatusFailed
	case p.ReviewedSHA != "" && p.HeadSHA == p.ReviewedSHA:
		if r := lastCompleted(*p); r != nil {
			p.Status = r.Status
		}
	case p.ReviewedSHA != "":
		p.Status = model.StatusOutdated
	default:
		p.Status = model.StatusIdle
	}
}

func failedAt(p model.PR) bool {
	r := p.LastReview()
	return r != nil && r.Status == model.StatusFailed && r.HeadSHA == p.HeadSHA && r.Error != "interrupted" && r.Error != "canceled"
}
