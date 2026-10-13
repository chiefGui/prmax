package model

import (
	"strconv"
	"time"
)

const (
	StatusIdle      = "idle"
	StatusOutdated  = "outdated"
	StatusQueued    = "queued"
	StatusReviewing = "reviewing"
	StatusClean     = "clean"
	StatusFindings  = "findings"
	StatusFailed    = "failed"
)

const (
	KindFull        = "full"
	KindIncremental = "incremental"
)

type PR struct {
	Repo        string    `json:"repo"`
	Number      int       `json:"number"`
	Title       string    `json:"title"`
	Body        string    `json:"body"`
	Author      string    `json:"author"`
	URL         string    `json:"url"`
	Branch      string    `json:"branch"`
	BaseBranch  string    `json:"baseBranch"`
	HeadSHA     string    `json:"headSha"`
	Draft       bool      `json:"draft"`
	ReviewedSHA string    `json:"reviewedSha"`
	Activity    int       `json:"activity"`
	Seen        int       `json:"seen"`
	Status      string    `json:"status"`
	UpdatedAt   time.Time `json:"updatedAt"`
	Reviews     []Review  `json:"reviews"`
}

func (p PR) Key() string { return Key(p.Repo, p.Number) }

func (p PR) LastReview() *Review {
	if len(p.Reviews) == 0 {
		return nil
	}
	return &p.Reviews[len(p.Reviews)-1]
}

type Review struct {
	ID         string    `json:"id"`
	Repo       string    `json:"repo"`
	Number     int       `json:"number"`
	Kind       string    `json:"kind"`
	Provider   string    `json:"provider"`
	Model      string    `json:"model"`
	Effort     string    `json:"effort"`
	Access     string    `json:"access"`
	Scope      string    `json:"scope,omitempty"`
	ModelLabel string    `json:"modelLabel"`
	Round      int       `json:"round"`
	HeadSHA    string    `json:"headSha"`
	SinceSHA   string    `json:"sinceSha,omitempty"`
	Status     string    `json:"status"`
	StartedAt  time.Time `json:"startedAt"`
	FinishedAt time.Time `json:"finishedAt,omitzero"`
	CostUSD    float64   `json:"costUsd"`
	Findings   []Finding `json:"findings"`
	Error      string    `json:"error,omitempty"`
	CommentURL string    `json:"commentUrl,omitempty"`
	NudgedAt   time.Time `json:"nudgedAt,omitzero"`
	NudgeError string    `json:"nudgeError,omitempty"`
}

type Finding struct {
	Category  string `json:"category"`
	File      string `json:"file"`
	Line      int    `json:"line"`
	Title     string `json:"title"`
	Text      string `json:"text"`
	StillOpen bool   `json:"stillOpen"`
}

var Categories = []struct{ ID, Label string }{
	{"bugs", "Bugs"},
	{"cleanup", "Cleanup"},
	{"performance", "Performance"},
	{"architecture", "Architecture"},
	{"tests", "Tests"},
	{"docs", "Docs"},
}

func (r Review) ByCategory(id string) []Finding {
	var out []Finding
	for _, f := range r.Findings {
		if f.Category == id {
			out = append(out, f)
		}
	}
	return out
}

type Forwarder struct {
	Repo      string    `json:"repo"`
	Connected bool      `json:"connected"`
	Error     string    `json:"error,omitempty"`
	LastEvent time.Time `json:"lastEvent,omitzero"`
}

type State struct {
	PRs        []PR        `json:"prs"`
	Forwarders []Forwarder `json:"forwarders"`
	Started    time.Time   `json:"started"`
}

type LogLine struct {
	ReviewID string    `json:"reviewId"`
	Time     time.Time `json:"time"`
	Kind     string    `json:"kind"`
	Text     string    `json:"text"`
}

func Key(repo string, number int) string {
	return repo + "#" + strconv.Itoa(number)
}

type ModelOption struct {
	ID            string   `json:"id"`
	Label         string   `json:"label"`
	Efforts       []string `json:"efforts"`
	DefaultEffort string   `json:"defaultEffort"`
}

type Provider struct {
	ID      string        `json:"id"`
	Label   string        `json:"label"`
	Models  []ModelOption `json:"models"`
	Default string        `json:"default"`
}

type Choice struct {
	Provider string            `json:"provider"`
	Models   map[string]string `json:"models"`
	Efforts  map[string]string `json:"efforts"`
	Access   string            `json:"access"`
	Scope    string            `json:"scope"`
	Nudge    bool              `json:"nudge"`
}

type Providers struct {
	List   []Provider `json:"list"`
	Choice Choice     `json:"choice"`
}

type ReviewRequest struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
	Effort   string `json:"effort"`
	Access   string `json:"access"`
	Scope    string `json:"scope"`
	Nudge    bool   `json:"nudge"`
}

type Commit struct {
	SHA   string `json:"sha"`
	Title string `json:"title"`
}

type Plan struct {
	Round       int      `json:"round"`
	Since       string   `json:"since,omitempty"`
	SinceRound  int      `json:"sinceRound,omitempty"`
	SinceIssues int      `json:"sinceIssues,omitempty"`
	Commits     []Commit `json:"commits,omitempty"`
	Rewritten   bool     `json:"rewritten,omitempty"`
	Unchanged   bool     `json:"unchanged,omitempty"`
}

func (p PR) Unread() bool { return p.Activity > p.Seen }

const (
	AccessFull     = "full"
	AccessReadOnly = "read-only"
)

var AccessModes = []string{AccessFull, AccessReadOnly}

const (
	ScopeChanges = "changes"
	ScopeWhole   = "whole PR"
)

var Scopes = []string{ScopeChanges, ScopeWhole}
