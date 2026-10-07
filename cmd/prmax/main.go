package main

import (
	"fmt"
	"os"
	"os/exec"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"prmax/internal/config"
	"prmax/internal/daemon"
	"prmax/internal/proc"
	"prmax/internal/tui"
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
		client := tui.NewClient(config.Default().Listen)
		if cfg, err := config.Load(); err == nil {
			client = tui.NewClient(cfg.Listen)
		}
		run := install
		if cmd == "uninstall" {
			run = uninstall
		}
		if err := run(client); err != nil {
			fail(err)
		}
		return
	}
	cfg, err := config.Load()
	if err != nil {
		fail(err)
	}
	client := tui.NewClient(cfg.Listen)
	switch cmd {
	case "daemon":
		if err := daemon.Run(cfg); err != nil {
			fail(err)
		}
	case "stop":
		if err := client.Shutdown(); err != nil {
			fail(fmt.Errorf("daemon not running"))
		}
		fmt.Println("daemon stopped")
	case "start":
		if err := ensureDaemon(client); err != nil {
			fail(err)
		}
		fmt.Println("daemon running on", cfg.Listen)
	case "":
		if err := ensureDaemon(client); err != nil {
			fail(err)
		}
		if _, err := tea.NewProgram(tui.New(client), tea.WithAltScreen(), tea.WithMouseCellMotion()).Run(); err != nil {
			fail(err)
		}
	default:
		fmt.Fprintln(os.Stderr, "usage: prmax [install|uninstall|start|stop|config|daemon]")
		os.Exit(2)
	}
}

func ensureDaemon(client *tui.Client) error {
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
