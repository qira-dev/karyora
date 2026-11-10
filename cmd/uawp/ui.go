package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"time"

	"github.com/qira-dev/karyora/internal/platform"
	"github.com/qira-dev/karyora/internal/runtime"
)

func openAppWindow(targetURL string) error {
	switch goruntime.GOOS {
	case "linux":
		// Prefer standalone desktop application window (--app mode) without browser tabs/address bar.
		candidates := []string{"brave-browser", "google-chrome", "chromium", "chromium-browser", "microsoft-edge"}
		for _, name := range candidates {
			if path, err := exec.LookPath(name); err == nil {
				cmd := exec.Command(path, fmt.Sprintf("--app=%s", targetURL), "--class=karyora", "--name=karyora")
				if err := cmd.Start(); err == nil {
					return nil
				}
			}
		}
		cmd := exec.Command("xdg-open", targetURL)
		return cmd.Start()
	case "darwin":
		cmd := exec.Command("open", targetURL)
		return cmd.Start()
	case "windows":
		candidates := []string{"msedge.exe", "chrome.exe"}
		for _, name := range candidates {
			if path, err := exec.LookPath(name); err == nil {
				cmd := exec.Command(path, fmt.Sprintf("--app=%s", targetURL))
				if err := cmd.Start(); err == nil {
					return nil
				}
			}
		}
		cmd := exec.Command("rundll32", "url.dll,FileProtocolHandler", targetURL)
		return cmd.Start()
	default:
		return fmt.Errorf("unsupported platform: %s", goruntime.GOOS)
	}
}

func openBrowser(targetURL string) error {
	return openAppWindow(targetURL)
}

// listenerAlive reports whether a TCP listener answers at addr.
func listenerAlive(addr string) bool {
	c, err := net.DialTimeout("tcp", addr, 500*time.Millisecond)
	if err != nil {
		return false
	}
	_ = c.Close()
	return true
}

func cmdUI(ctx context.Context, flags cliFlags, out *printer) int {
	ds, f := resolvePaths(flags)
	if f != nil {
		out.failure(f)
		return int(f.Exit)
	}

	// 1. If daemon is already running, open UI pointing to it.
	// Ownership/address records can outlive a killed process, so liveness is
	// proven by the loopback listener actually accepting a connection.
	if rec, _, err := platform.OwnerCredentials(ds.Runtime); err == nil && rec.PID > 0 {
		addrBytes, err := os.ReadFile(filepath.Join(ds.Runtime, "http_address"))
		addr := strings.TrimSpace(string(addrBytes))
		if err == nil && addr != "" && listenerAlive(addr) {
			targetURL := "http://" + addr + "/"
			tokenBytes, _ := os.ReadFile(filepath.Join(ds.Runtime, "bootstrap_token"))
			token := strings.TrimSpace(string(tokenBytes))
			if out.json {
				out.report(map[string]any{
					"status": "running",
					"url":    targetURL,
					"token":  token,
					"pid":    rec.PID,
				})
			} else {
				fmt.Printf("[karyora] Connected to running instance (PID %d)\n", rec.PID)
				fmt.Printf("[karyora] Web UI: %s\n", targetURL)
				if token != "" {
					fmt.Printf("[karyora] Bootstrap token: %s\n", token)
				}
				fmt.Println("[karyora] Opening application window...")
			}
			_ = openAppWindow(targetURL)
			return int(runtime.ExitOK)
		}
	}

	// 2. Otherwise enable HTTP transport and start the server.
	if flags.overrides == nil {
		flags.overrides = make(map[string]any)
	}
	flags.overrides["transport.local_http.enabled"] = 1
	if flags.overrides["transport.local_http.port"] == nil {
		flags.overrides["transport.local_http.port"] = 27210
	}

	go func() {
		for i := 0; i < 20; i++ {
			time.Sleep(250 * time.Millisecond)
			addrBytes, err := os.ReadFile(filepath.Join(ds.Runtime, "http_address"))
			if err == nil && len(addrBytes) > 0 {
				addr := strings.TrimSpace(string(addrBytes))
				targetURL := "http://" + addr + "/"
				_ = openAppWindow(targetURL)
				return
			}
		}
	}()

	return cmdStart(ctx, flags, out)
}
