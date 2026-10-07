package daemon

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"prmax/internal/model"
)

const maxReviewsPerPR = 20

type Event struct {
	Type string
	Data []byte
}

type Store struct {
	mu         sync.Mutex
	dir        string
	prs        map[string]*model.PR
	forwarders map[string]*model.Forwarder
	started    time.Time
	subs       map[chan Event]struct{}
	dirty      chan struct{}
	logMu      sync.Mutex
	logFiles   map[string]*os.File
}

func NewStore(dir string) (*Store, error) {
	if err := os.MkdirAll(filepath.Join(dir, "logs"), 0o755); err != nil {
		return nil, err
	}
	s := &Store{
		dir:        dir,
		prs:        map[string]*model.PR{},
		forwarders: map[string]*model.Forwarder{},
		started:    time.Now(),
		subs:       map[chan Event]struct{}{},
		dirty:      make(chan struct{}, 1),
		logFiles:   map[string]*os.File{},
	}
	if b, err := os.ReadFile(s.statePath()); err == nil {
		var st model.State
		if json.Unmarshal(b, &st) == nil {
			for i := range st.PRs {
				p := st.PRs[i]
				for j := range p.Reviews {
					if p.Reviews[j].Status == model.StatusReviewing {
						p.Reviews[j].Status = model.StatusFailed
						p.Reviews[j].Error = "interrupted"
					}
				}
				if p.Status == model.StatusReviewing || p.Status == model.StatusQueued {
					p.Status = model.StatusIdle
				}
				settle(&p)
				s.prs[p.Key()] = &p
			}
		}
	}
	go s.flushLoop()
	return s, nil
}

func (s *Store) statePath() string { return filepath.Join(s.dir, "state.json") }

func (s *Store) LogPath(id string) string { return filepath.Join(s.dir, "logs", id+".jsonl") }

func (s *Store) PromptPath(id string) string { return filepath.Join(s.dir, "logs", id+".prompt.md") }

func (s *Store) Update(fn func(prs map[string]*model.PR)) {
	s.mu.Lock()
	fn(s.prs)
	s.mu.Unlock()
	s.changed()
}

func (s *Store) Get(key string) (model.PR, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.prs[key]
	if !ok {
		return model.PR{}, false
	}
	return clonePR(*p), true
}

func (s *Store) SetForwarder(f model.Forwarder) {
	s.mu.Lock()
	cur, ok := s.forwarders[f.Repo]
	if ok && f.LastEvent.IsZero() {
		f.LastEvent = cur.LastEvent
	}
	s.forwarders[f.Repo] = &f
	s.mu.Unlock()
	s.changed()
}

func (s *Store) TouchForwarder(repo string) {
	s.mu.Lock()
	if f, ok := s.forwarders[repo]; ok {
		f.LastEvent = time.Now()
	}
	s.mu.Unlock()
	s.changed()
}

func (s *Store) Snapshot() model.State {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := model.State{Started: s.started}
	for _, p := range s.prs {
		st.PRs = append(st.PRs, clonePR(*p))
	}
	sort.Slice(st.PRs, func(i, j int) bool { return st.PRs[i].UpdatedAt.After(st.PRs[j].UpdatedAt) })
	for _, f := range s.forwarders {
		st.Forwarders = append(st.Forwarders, *f)
	}
	sort.Slice(st.Forwarders, func(i, j int) bool { return st.Forwarders[i].Repo < st.Forwarders[j].Repo })
	return st
}

func (s *Store) Subscribe() chan Event {
	ch := make(chan Event, 256)
	s.mu.Lock()
	s.subs[ch] = struct{}{}
	s.mu.Unlock()
	return ch
}

func (s *Store) Unsubscribe(ch chan Event) {
	s.mu.Lock()
	delete(s.subs, ch)
	s.mu.Unlock()
}

func (s *Store) publish(e Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for ch := range s.subs {
		select {
		case ch <- e:
		default:
		}
	}
}

func (s *Store) changed() {
	select {
	case s.dirty <- struct{}{}:
	default:
	}
}

func (s *Store) flushLoop() {
	for range s.dirty {
		time.Sleep(30 * time.Millisecond)
		st := s.Snapshot()
		b, err := json.Marshal(st)
		if err != nil {
			continue
		}
		s.publish(Event{Type: "state", Data: b})
		tmp := s.statePath() + ".tmp"
		if os.WriteFile(tmp, b, 0o644) == nil {
			os.Rename(tmp, s.statePath())
		}
	}
}

func (s *Store) AppendLog(l model.LogLine) {
	b, err := json.Marshal(l)
	if err != nil {
		return
	}
	s.logMu.Lock()
	f, ok := s.logFiles[l.ReviewID]
	if !ok {
		f, err = os.OpenFile(s.LogPath(l.ReviewID), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err == nil {
			s.logFiles[l.ReviewID] = f
		}
	}
	if f != nil {
		f.Write(append(b, '\n'))
	}
	s.logMu.Unlock()
	s.publish(Event{Type: "log", Data: b})
}

func (s *Store) CloseLog(id string) {
	s.logMu.Lock()
	if f, ok := s.logFiles[id]; ok {
		f.Close()
		delete(s.logFiles, id)
	}
	s.logMu.Unlock()
}

func (s *Store) ReadLog(id string) []model.LogLine {
	f, err := os.Open(s.LogPath(id))
	if err != nil {
		return nil
	}
	defer f.Close()
	var out []model.LogLine
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	for sc.Scan() {
		var l model.LogLine
		if json.Unmarshal(sc.Bytes(), &l) == nil {
			out = append(out, l)
		}
	}
	return out
}

func clonePR(p model.PR) model.PR {
	p.Reviews = append([]model.Review(nil), p.Reviews...)
	return p
}

func trimReviews(p *model.PR) {
	if len(p.Reviews) > maxReviewsPerPR {
		p.Reviews = p.Reviews[len(p.Reviews)-maxReviewsPerPR:]
	}
}
