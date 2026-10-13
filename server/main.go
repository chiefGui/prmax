package main

import (
	"fmt"
	"net/http"
	"os"
	"time"

	"prmax/config"
	"prmax/daemon"
)

const (
	appExe    = "prmax.exe"
	serverExe = "prmax-server.exe"
)

func main() {
	cmd := ""
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}
	listen := config.Default().Listen
	cfg, cfgErr := config.Load()
	if cfgErr == nil {
		listen = cfg.Listen
	}
	var err error
	switch cmd {
	case "":
		if cfgErr != nil {
			fail(cfgErr)
		}
		err = daemon.Run(cfg)
	case "install":
		err = install(listen)
	case "uninstall":
		err = uninstall(listen)
	case "stop":
		if shutdown(listen) != nil {
			err = fmt.Errorf("server not running")
		}
	default:
		fmt.Fprintln(os.Stderr, "usage: prmax-server [install|uninstall|stop]")
		os.Exit(2)
	}
	if err != nil {
		fail(err)
	}
}

var client = &http.Client{Timeout: 5 * time.Second}

func ping(listen string) error {
	resp, err := client.Get("http://" + listen + "/api/state")
	if err != nil {
		return err
	}
	return resp.Body.Close()
}

func shutdown(listen string) error {
	resp, err := client.Post("http://"+listen+"/api/shutdown", "", nil)
	if err != nil {
		return err
	}
	return resp.Body.Close()
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
