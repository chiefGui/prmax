package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"prmax/internal/client"
	"prmax/internal/config"
	"prmax/internal/daemon"
	"prmax/internal/proc"
)

func main() {
	cmd := ""
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}
	if cmd == "config" {
		fmt.Println(config.Path())
		return
	}
	if cmd == "install" || cmd == "uninstall" {
		api := client.New(config.Default().Listen)
		if cfg, err := config.Load(); err == nil {
			api = client.New(cfg.Listen)
		}
		run := install
		if cmd == "uninstall" {
			run = uninstall
		}
		if err := run(api); err != nil {
			fail(err)
		}
		return
	}
	cfg, err := config.Load()
	if err != nil {
		fail(err)
	}
	api := client.New(cfg.Listen)
	switch cmd {
	case "daemon":
		if err := daemon.Run(cfg); err != nil {
			fail(err)
		}
	case "stop":
		if err := api.Shutdown(); err != nil {
			fail(fmt.Errorf("daemon not running"))
		}
		fmt.Println("daemon stopped")
	case "start":
		if err := ensureDaemon(api); err != nil {
			fail(err)
		}
		fmt.Println("daemon running on", cfg.Listen)
	case "":
		if err := ensureDaemon(api); err != nil {
			fail(err)
		}
		if err := openDesktop(); err != nil {
			fail(err)
		}
	default:
		fmt.Fprintln(os.Stderr, "usage: prmax [install|uninstall|start|stop|config|daemon]")
		os.Exit(2)
	}
}

const desktopExe = "prmax-desktop.exe"

func openDesktop() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	return proc.Open(filepath.Join(filepath.Dir(exe), desktopExe))
}

func ensureDaemon(client *client.Client) error {
	if client.Ping() == nil {
		return nil
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	c := exec.Command(exe, "daemon")
	proc.Detach(c)
	if err := c.Start(); err != nil {
		c = exec.Command(exe, "daemon")
		proc.DetachFallback(c)
		if err := c.Start(); err != nil {
			return err
		}
	}
	c.Process.Release()
	for i := 0; i < 50; i++ {
		time.Sleep(100 * time.Millisecond)
		if client.Ping() == nil {
			return nil
		}
	}
	return fmt.Errorf("daemon did not start; see %s", config.Dir())
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
