package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"time"

	"prmax/internal/model"
)

type webhookPR struct {
	Action      string `json:"action"`
	PullRequest struct {
		Number    int       `json:"number"`
		Title     string    `json:"title"`
		Body      string    `json:"body"`
		HTMLURL   string    `json:"html_url"`
		Draft     bool      `json:"draft"`
		State     string    `json:"state"`
		UpdatedAt time.Time `json:"updated_at"`
		User      struct {
			Login string `json:"login"`
		} `json:"user"`
		Head struct {
			SHA string `json:"sha"`
			Ref string `json:"ref"`
		} `json:"head"`
		Base struct {
			Ref string `json:"ref"`
		} `json:"base"`
	} `json:"pull_request"`
}

func (d *Daemon) routes(shutdown context.CancelFunc) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /webhook/{owner}/{name}", d.handleWebhook)
	mux.HandleFunc("GET /api/state", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, d.store.Snapshot())
	})
	mux.HandleFunc("GET /api/events", d.handleEvents)
	mux.HandleFunc("GET /api/providers", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, d.providers())
	})
	mux.HandleFunc("GET /api/reviews/{id}/log", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, d.store.ReadLog(r.PathValue("id")))
	})
	mux.HandleFunc("POST /api/prs/{owner}/{name}/{n}/{action}", d.handleAction)
	mux.HandleFunc("GET /api/prs/{owner}/{name}/{n}/plan", func(w http.ResponseWriter, r *http.Request) {
		n, err := strconv.Atoi(r.PathValue("n"))
		if err != nil {
			http.Error(w, "bad number", http.StatusBadRequest)
			return
		}
		pl, err := d.plan(r.Context(), model.Key(r.PathValue("owner")+"/"+r.PathValue("name"), n))
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		writeJSON(w, pl)
	})
	mux.HandleFunc("POST /api/shutdown", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
		go func() {
			time.Sleep(100 * time.Millisecond)
			shutdown()
		}()
	})
	return mux
}

func (d *Daemon) handleWebhook(w http.ResponseWriter, r *http.Request) {
	repo := r.PathValue("owner") + "/" + r.PathValue("name")
	if _, ok := d.cfg.Repo(repo); !ok {
		http.Error(w, "unknown repo", http.StatusNotFound)
		return
	}
	d.store.TouchForwarder(repo)
	if r.Header.Get("X-GitHub-Event") != "pull_request" {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 25<<20))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var ev webhookPR
	if err := json.Unmarshal(body, &ev); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusNoContent)
	pr := ev.PullRequest
	key := model.Key(repo, pr.Number)
	log.Printf("webhook %s %s", key, ev.Action)
	if ev.Action == "closed" || pr.State == "closed" {
		d.store.Update(func(m map[string]*model.PR) { delete(m, key) })
		d.cancel(key)
		return
	}
	d.store.Update(func(m map[string]*model.PR) {
		p, ok := m[key]
		if !ok {
			p = &model.PR{Repo: repo, Number: pr.Number, Status: model.StatusIdle, Activity: 1}
			m[key] = p
		}
		p.Title = pr.Title
		p.Body = pr.Body
		p.URL = pr.HTMLURL
		p.Draft = pr.Draft
		p.Author = pr.User.Login
		p.Branch = pr.Head.Ref
		p.BaseBranch = pr.Base.Ref
		setHead(p, pr.Head.SHA)
		if pr.UpdatedAt.After(p.UpdatedAt) {
			p.UpdatedAt = pr.UpdatedAt
		}
		settle(p)
	})
}

func (d *Daemon) handleEvents(w http.ResponseWriter, r *http.Request) {
	fl, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	ch := d.store.Subscribe()
	defer d.store.Unsubscribe(ch)
	b, _ := json.Marshal(d.store.Snapshot())
	fmt.Fprintf(w, "event: state\ndata: %s\n\n", b)
	fl.Flush()
	ping := time.NewTicker(15 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ping.C:
			fmt.Fprint(w, ": ping\n\n")
			fl.Flush()
		case e := <-ch:
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", e.Type, e.Data)
			fl.Flush()
		}
	}
}

func (d *Daemon) handleAction(w http.ResponseWriter, r *http.Request) {
	repo := r.PathValue("owner") + "/" + r.PathValue("name")
	n, err := strconv.Atoi(r.PathValue("n"))
	if err != nil {
		http.Error(w, "bad number", http.StatusBadRequest)
		return
	}
	key := model.Key(repo, n)
	if _, ok := d.store.Get(key); !ok {
		http.Error(w, "unknown pr", http.StatusNotFound)
		return
	}
	switch r.PathValue("action") {
	case "review":
		var req model.ReviewRequest
		if err := json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&req); err != nil {
			http.Error(w, "bad request: "+err.Error(), http.StatusBadRequest)
			return
		}
		if err := d.request(key, req); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
	case "seen":
		d.store.Update(func(m map[string]*model.PR) {
			if p, ok := m[key]; ok {
				p.Seen = p.Activity
			}
		})
	case "cancel":
		d.cancel(key)
	case "refresh":
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()
			g, err := d.viewPR(ctx, repo, n)
			if err != nil {
				log.Printf("refresh %s: %v", key, err)
				return
			}
			d.store.Update(func(m map[string]*model.PR) {
				if p, ok := m[key]; ok {
					g.apply(p, repo)
					settle(p)
				}
			})
		}()
	default:
		http.Error(w, "unknown action", http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
