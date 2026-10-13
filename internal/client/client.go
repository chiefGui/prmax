package client

import (
	"net/http"
	"time"
)

type Client struct {
	base string
	http *http.Client
}

func New(listen string) *Client {
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

func (c *Client) Shutdown() error {
	resp, err := c.http.Post(c.base+"/api/shutdown", "", nil)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}
