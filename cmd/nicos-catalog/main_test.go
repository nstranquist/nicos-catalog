package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestVersionExpectation(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run(context.Background(), []string{"--json", "version", "--expect", "v0.1.1"}, &stdout, &stderr); code != 0 {
		t.Fatalf("run returned %d: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"version": "v0.1.1"`) {
		t.Fatalf("unexpected output: %s", stdout.String())
	}
}

func TestSyntheticDemoExcludesPrivateEntity(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run(context.Background(), []string{"--json", "demo"}, &stdout, &stderr); code != 0 {
		t.Fatalf("run returned %d: %s", code, stderr.String())
	}
	if strings.Contains(stdout.String(), "telemetry.private-sample") || strings.Contains(stdout.String(), "never projected") {
		t.Fatalf("demo public output leaked private entity: %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "system.orchard") {
		t.Fatalf("demo output missing public entity: %s", stdout.String())
	}
}
