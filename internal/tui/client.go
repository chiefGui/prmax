package tui

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"prmax/internal/model"
)

type Client struct {
	base string
	http *http.Client
}

func NewClient(listen string) *Client {
	return &Client{base: "http://" + listen, http: &http.Client{Timeout: 5 * time.Second}}
}

func (c *Client) Ping() error {
	resp, err := c.http.Get(c.base + "/api/state")
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

func (c *Client) Log(id string) ([]model.LogLine, error) {
	resp, err := c.http.Get(c.base + "/api/reviews/" + id + "/log")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var out []model.LogLine
	return out, json.NewDecoder(resp.Body).Decode(&out)
}

func (c *Client) Action(repo string, number int, action string) error {
	resp, err := c.http.Post(fmt.Sprintf("%s/api/prs/%s/%d/%s", c.base, repo, number, action), "", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		if msg := strings.TrimSpace(string(b)); msg != "" {
			return fmt.Errorf("%s", msg)
		}
		return fmt.Errorf("%s: %s", action, resp.Status)
	}
	return nil
}

func (c *Client) Shutdown() error {
	resp, err := c.http.Post(c.base+"/api/shutdown", "", nil)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

type streamMsg struct {
	state *model.State
	log   *model.LogLine
	err   error
}

func (c *Client) Stream(out chan<- streamMsg) {
	for {
		c.streamOnce(out)
		time.Sleep(time.Second)
	}
}

func (c *Client) streamOnce(out chan<- streamMsg) {
	resp, err := (&http.Client{}).Get(c.base + "/api/events")
	if err != nil {
		out <- streamMsg{err: err}
		return
	}
	defer resp.Body.Close()
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 1<<20), 1<<26)
	var event string
	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, "event: "):
			event = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			data := []byte(strings.TrimPrefix(line, "data: "))
			switch event {
			case "state":
				var st model.State
				if json.Unmarshal(data, &st) == nil {
					out <- streamMsg{state: &st}
				}
			case "log":
				var l model.LogLine
				if json.Unmarshal(data, &l) == nil {
					out <- streamMsg{log: &l}
				}
			}
		}
	}
	out <- streamMsg{err: fmt.Errorf("daemon disconnected")}
}

func (c *Client) Providers() (model.Providers, error) {
	var out model.Providers
	resp, err := c.http.Get(c.base + "/api/providers")
	if err != nil {
		return out, err
	}
	defer resp.Body.Close()
	return out, json.NewDecoder(resp.Body).Decode(&out)
}

func (c *Client) Review(repo string, number int, req model.ReviewRequest) error {
	b, _ := json.Marshal(req)
	resp, err := c.http.Post(fmt.Sprintf("%s/api/prs/%s/%d/review", c.base, repo, number), "application/json", bytes.NewReader(b))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("%s", strings.TrimSpace(string(msg)))
	}
	return nil
}

func (c *Client) Plan(repo string, number int) (model.Plan, error) {
	var out model.Plan
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Get(fmt.Sprintf("%s/api/prs/%s/%d/plan", c.base, repo, number))
	if err != nil {
		return out, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return out, fmt.Errorf("%s", strings.TrimSpace(string(msg)))
	}
	return out, json.NewDecoder(resp.Body).Decode(&out)
}

func (c *Client) Prompt(id string) (string, error) {
	resp, err := c.http.Get(c.base + "/api/reviews/" + id + "/prompt")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return "", fmt.Errorf("no prompt saved for this review")
	}
	b, err := io.ReadAll(resp.Body)
	return string(b), err
}
