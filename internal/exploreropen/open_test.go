package exploreropen

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestPlatformCommands(t *testing.T) {
	url := "http://127.0.0.1:7788"
	tests := []struct {
		goos, name string
		args       []string
	}{
		{"darwin", "open", []string{url}},
		{"linux", "xdg-open", []string{url}},
		{"windows", "rundll32", []string{"url.dll,FileProtocolHandler", url}},
	}
	for _, tt := range tests {
		name, args, err := Command(tt.goos, url)
		if err != nil || name != tt.name || !reflect.DeepEqual(args, tt.args) {
			t.Fatalf("%s = %s %v %v", tt.goos, name, args, err)
		}
	}
	if _, _, err := Command("plan9", url); err == nil {
		t.Fatal("unsupported platform succeeded")
	}
}

func TestURLValidation(t *testing.T) {
	for _, value := range []string{"http://127.0.0.1:1", "http://[::1]:2", "http://localhost:3"} {
		if err := validate(value); err != nil {
			t.Fatalf("%s: %v", value, err)
		}
	}
	for _, value := range []string{"https://127.0.0.1", "http://example.com", "http://user@localhost", ":bad"} {
		if err := validate(value); err == nil {
			t.Fatalf("unsafe URL accepted: %s", value)
		}
	}
}

func TestOpenUsesValidatedLauncher(t *testing.T) {
	previous := startCommand
	defer func() { startCommand = previous }()
	called := false
	startCommand = func(_ context.Context, name string, args ...string) error {
		called = name != "" && len(args) > 0
		return nil
	}
	if err := Open(context.Background(), "http://127.0.0.1:7788"); err != nil || !called {
		t.Fatalf("open = %v called=%v", err, called)
	}
	if err := Open(context.Background(), "http://example.com"); err == nil {
		t.Fatal("unsafe Open succeeded")
	}
	want := errors.New("start failed")
	startCommand = func(context.Context, string, ...string) error { return want }
	if err := Open(context.Background(), "http://127.0.0.1:7788"); !errors.Is(err, want) {
		t.Fatalf("start error = %v", err)
	}
}
