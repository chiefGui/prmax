package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"prmax/internal/config"
	"prmax/internal/proc"
	"prmax/internal/tui"
)

func install(client *tui.Client) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	dst := filepath.Join(config.BinDir(), "prmax.exe")
	if !samePath(exe, dst) {
		if client.Ping() == nil {
			client.Shutdown()
			time.Sleep(2 * time.Second)
		}
		if err := os.MkdirAll(config.BinDir(), 0o755); err != nil {
			return err
		}
		old := dst + ".old"
		os.Remove(old)
		if _, err := os.Stat(dst); err == nil {
			if err := os.Rename(dst, old); err != nil {
				return err
			}
		}
		if err := copyFile(exe, dst); err != nil {
			return err
		}
		os.Remove(old)
	}
	if err := addToPath(config.BinDir()); err != nil {
		return err
	}
	if _, err := config.Load(); err != nil {
		fmt.Println(err)
	}
	fmt.Println("installed", dst)
	fmt.Println("config   ", config.Path())
	fmt.Println("open a new terminal and run: prmax")
	return nil
}

func uninstall(client *tui.Client) error {
	if client.Ping() == nil {
		client.Shutdown()
		time.Sleep(2 * time.Second)
	}
	root := config.Root()
	if cfg, err := config.Load(); err == nil {
		for _, r := range cfg.Repos {
			cleanRepo(r, root)
		}
	}
	if err := removeFromPath(config.BinDir()); err != nil {
		fmt.Println("path:", err)
	}
	exe, _ := os.Executable()
	if strings.HasPrefix(strings.ToLower(exe), strings.ToLower(root)+string(os.PathSeparator)) {
		c := exec.Command("cmd", "/c", fmt.Sprintf(`ping -n 3 127.0.0.1 >nul & rmdir /s /q "%s"`, root))
		proc.Detach(c)
		if err := c.Start(); err != nil {
			return err
		}
	} else if err := os.RemoveAll(root); err != nil {
		return err
	}
	fmt.Println("prmax uninstalled")
	return nil
}

func cleanRepo(r config.Repo, root string) {
	git := func(args ...string) string {
		c := exec.Command("git", append([]string{"-C", r.Path}, args...)...)
		proc.Hide(c)
		out, _ := c.Output()
		return string(out)
	}
	prefix := strings.ToLower(filepath.Base(r.Name)) + "-"
	entries, _ := os.ReadDir(filepath.Join(root, "worktrees"))
	for _, e := range entries {
		if e.IsDir() && strings.HasPrefix(e.Name(), prefix) {
			git("worktree", "remove", "--force", filepath.Join(root, "worktrees", e.Name()))
		}
	}
	git("worktree", "prune")
	for _, ref := range strings.Fields(git("for-each-ref", "--format=%(refname)", "refs/prmax")) {
		git("update-ref", "-d", ref)
	}
}

func samePath(a, b string) bool {
	ra, err1 := filepath.Abs(a)
	rb, err2 := filepath.Abs(b)
	return err1 == nil && err2 == nil && strings.EqualFold(ra, rb)
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

func userPath() (registry.Key, []string, error) {
	k, err := registry.OpenKey(registry.CURRENT_USER, "Environment", registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return 0, nil, err
	}
	v, _, err := k.GetStringValue("Path")
	if err != nil && err != registry.ErrNotExist {
		k.Close()
		return 0, nil, err
	}
	var parts []string
	for _, p := range strings.Split(v, ";") {
		if p != "" {
			parts = append(parts, p)
		}
	}
	return k, parts, nil
}

func addToPath(dir string) error {
	k, parts, err := userPath()
	if err != nil {
		return err
	}
	defer k.Close()
	for _, p := range parts {
		if strings.EqualFold(strings.TrimRight(p, `\`), strings.TrimRight(dir, `\`)) {
			return nil
		}
	}
	parts = append([]string{dir}, parts...)
	if err := k.SetExpandStringValue("Path", strings.Join(parts, ";")); err != nil {
		return err
	}
	broadcastEnv()
	return nil
}

func removeFromPath(dir string) error {
	k, parts, err := userPath()
	if err != nil {
		return err
	}
	defer k.Close()
	var keep []string
	for _, p := range parts {
		if !strings.EqualFold(strings.TrimRight(p, `\`), strings.TrimRight(dir, `\`)) {
			keep = append(keep, p)
		}
	}
	if len(keep) == len(parts) {
		return nil
	}
	if err := k.SetExpandStringValue("Path", strings.Join(keep, ";")); err != nil {
		return err
	}
	broadcastEnv()
	return nil
}

func broadcastEnv() {
	env, _ := syscall.UTF16PtrFromString("Environment")
	fn := windows.NewLazySystemDLL("user32.dll").NewProc("SendMessageTimeoutW")
	var result uintptr
	fn.Call(0xffff, 0x001A, 0, uintptr(unsafe.Pointer(env)), 0x0002, 5000, uintptr(unsafe.Pointer(&result)))
}
