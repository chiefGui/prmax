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
	ID         string     `json:"id"`
	Repo       string     `json:"repo"`
	Number     int        `json:"number"`
	Kind       string     `json:"kind"`
	Provider   string     `json:"provider"`
	Model      string     `json:"model"`
	Effort     string     `json:"effort"`
	ModelLabel string     `json:"modelLabel"`
	HeadSHA    string     `json:"headSha"`
	SinceSHA   string     `json:"sinceSha,omitempty"`
	Status     string     `json:"status"`
	StartedAt  time.Time  `json:"startedAt"`
	FinishedAt time.Time  `json:"finishedAt,omitzero"`
	CostUSD    float64    `json:"costUsd"`
	Summary    string     `json:"summary"`
	Findings   []Finding  `json:"findings"`
	Previous   []Previous `json:"previous"`
	Error      string     `json:"error,omitempty"`
	CommentURL string     `json:"commentUrl,omitempty"`
}

type Finding struct {
	Severity string `json:"severity"`
	File     string `json:"file"`
	Line     int    `json:"line"`
	Title    string `json:"title"`
	Detail   string `json:"detail"`
}

type Previous struct {
	Title  string `json:"title"`
	Status string `json:"status"`
	Note   string `json:"note"`
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
}

type Providers struct {
	List   []Provider `json:"list"`
	Choice Choice     `json:"choice"`
}

type ReviewRequest struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
	Effort   string `json:"effort"`
}
