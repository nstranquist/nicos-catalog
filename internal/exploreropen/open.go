// Package exploreropen opens a verified loopback Explorer URL with the host's
// standard browser launcher.
package exploreropen

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"os/exec"
	"runtime"
)

var startCommand = func(ctx context.Context, name string, args ...string) error {
	//nolint:gosec // Command allowlists the launcher and validate restricts the URL to loopback.
	return exec.CommandContext(ctx, name, args...).Start()
}

// Open starts the platform browser for a validated loopback Explorer URL.
func Open(ctx context.Context, rawURL string) error {
	if err := validate(rawURL); err != nil {
		return err
	}
	name, args, err := Command(runtime.GOOS, rawURL)
	if err != nil {
		return err
	}
	return startCommand(ctx, name, args...)
}

// Command returns the platform launcher without starting a process.
func Command(goos, rawURL string) (string, []string, error) {
	switch goos {
	case "darwin":
		return "open", []string{rawURL}, nil
	case "linux":
		return "xdg-open", []string{rawURL}, nil
	case "windows":
		return "rundll32", []string{"url.dll,FileProtocolHandler", rawURL}, nil
	default:
		return "", nil, fmt.Errorf("automatic browser opening is not supported on this platform")
	}
}

func validate(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "http" || parsed.Hostname() == "" || parsed.User != nil {
		return fmt.Errorf("browser URL must be a loopback HTTP URL")
	}
	host := parsed.Hostname()
	if host == "localhost" {
		return nil
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("browser URL must be a loopback HTTP URL")
	}
	return nil
}
