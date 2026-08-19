package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nstranquist/nicos-catalog/internal/explorercontract"
)

func TestInitCommandReceiptsAndRefusal(t *testing.T) {
	root := t.TempDir()
	code, stdout, stderr := exec(t, "--json", "init", "--root", root, "--template", "sample", "--dry-run")
	if code != 0 {
		t.Fatalf("init dry-run = %d %s", code, stderr)
	}
	var envelope explorercontract.Envelope
	if err := json.Unmarshal([]byte(stdout), &envelope); err != nil || !envelope.OK || envelope.Command != "init" {
		t.Fatalf("receipt = %+v err=%v", envelope, err)
	}
	code, _, stderr = exec(t, "init", "--root", root, "--template", "minimal")
	if code != 0 {
		t.Fatalf("init = %d %s", code, stderr)
	}
	if _, err := os.Stat(filepath.Join(root, "catalog", "example.md")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "catalog", "example.md"), []byte("caller"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, stdout, _ = exec(t, "--json", "init", "--root", root)
	if code != 1 || !strings.Contains(stdout, `"ok": false`) || strings.Contains(stdout, root) {
		t.Fatalf("refusal = %d %s", code, stdout)
	}
	if code, _, _ := exec(t, "init", "--template", "bad", "--root", root); code != 1 {
		t.Fatalf("bad template = %d", code)
	}
	if code, _, _ := exec(t, "init", "extra"); code != 2 {
		t.Fatalf("extra init arg = %d", code)
	}
}

func TestExportExplorerCommand(t *testing.T) {
	root := reindexed(t)
	temp, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(temp, "explorer")
	code, stdout, stderr := exec(t, "--json", "--root", root, "export", "explorer", "--out", out, "--visibility", "public")
	if code != 0 {
		t.Fatalf("export = %d %s", code, stderr)
	}
	var envelope explorercontract.Envelope
	if err := json.Unmarshal([]byte(stdout), &envelope); err != nil || !envelope.OK || envelope.ProjectionMode != explorercontract.ProjectionPublic {
		t.Fatalf("export receipt = %+v err=%v", envelope, err)
	}
	if _, err := os.Stat(filepath.Join(out, "data", "manifest.json")); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"export"}, {"export", "other"}, {"export", "explorer", "--out", out}, {"export", "explorer", "--out", out, "--visibility", "private"}} {
		if code, _, _ := exec(t, append([]string{"--root", root}, args...)...); code == 0 {
			t.Fatalf("invalid export succeeded: %v", args)
		}
	}
}

func TestServeAndDemoUICompiledPaths(t *testing.T) {
	root := reindexed(t)
	ctx, cancel := context.WithCancel(context.Background())
	reader, writer := io.Pipe()
	done := make(chan int, 1)
	go func() { done <- run(ctx, []string{"--root", root, "serve"}, writer, io.Discard); _ = writer.Close() }()
	line, err := bufio.NewReader(reader).ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	url := strings.TrimSpace(strings.TrimPrefix(line, "Explorer: "))
	if !strings.HasPrefix(url, "http://127.0.0.1:") {
		t.Fatalf("serve line = %q", line)
	}
	client := &http.Client{Timeout: 2 * time.Second}
	response, err := client.Get(url + "/api/v1/status")
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("serve HTTP = %d", response.StatusCode)
	}
	cancel()
	select {
	case code := <-done:
		if code != 0 {
			t.Fatalf("serve exit = %d", code)
		}
	case <-time.After(6 * time.Second):
		t.Fatal("serve did not stop")
	}

	demoCtx, demoCancel := context.WithCancel(context.Background())
	demoReader, demoWriter := io.Pipe()
	demoDone := make(chan int, 1)
	go func() {
		demoDone <- run(demoCtx, []string{"demo", "--ui"}, demoWriter, io.Discard)
		_ = demoWriter.Close()
	}()
	demoLine, err := bufio.NewReader(demoReader).ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(demoLine, "synthetic Explorer: http://127.0.0.1:") {
		t.Fatalf("demo line = %q", demoLine)
	}
	demoCancel()
	select {
	case code := <-demoDone:
		if code != 0 {
			t.Fatalf("demo exit = %d", code)
		}
	case <-time.After(6 * time.Second):
		t.Fatal("demo did not stop")
	}

	if code, _, stderr := exec(t, "--root", writeDemoCorpus(t), "serve"); code != 1 || !strings.Contains(stderr, "run nicos-catalog reindex") {
		t.Fatalf("stale serve = %d %s", code, stderr)
	}
	if code, _, _ := exec(t, "--root", root, "serve", "--listen", "0.0.0.0:8080"); code != 1 {
		t.Fatalf("unsafe serve = %d", code)
	}
	if code, _, _ := exec(t, "--root", root, "serve", "extra"); code != 2 {
		t.Fatalf("serve extra = %d", code)
	}
}

func TestMCPCommandStdioAndUsage(t *testing.T) {
	root := reindexed(t)
	previous := commandStdin
	defer func() { commandStdin = previous }()
	commandStdin = strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}` + "\n")
	code, stdout, stderr := exec(t, "--root", root, "mcp", "--stdio")
	if code != 0 || !strings.Contains(stdout, `"protocolVersion"`) {
		t.Fatalf("mcp = %d %s %s", code, stdout, stderr)
	}
	if code, _, _ := exec(t, "--root", root, "mcp"); code != 2 {
		t.Fatalf("mcp without stdio = %d", code)
	}
	if code, _, _ := exec(t, "--json", "--root", root, "mcp", "--stdio"); code != 2 {
		t.Fatalf("json mcp = %d", code)
	}
}

func TestCommandReceiptEncodeFailure(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := writeCommandReceipt(&stdout, &stderr, "test", explorercontract.ProjectionLocal, "", func() {}, nil); code != 1 {
		t.Fatalf("encode failure = %d", code)
	}
}
