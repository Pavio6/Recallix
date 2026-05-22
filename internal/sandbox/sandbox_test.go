package sandbox

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestExecutorRejectsDisabledMode(t *testing.T) {
	exec := New("disabled", time.Second, "")
	if exec.Enabled() {
		t.Fatal("disabled sandbox should not be enabled")
	}
}

func TestExecutorRunsSafeScript(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "hello.sh")
	if err := os.WriteFile(script, []byte("#!/bin/bash\necho hello\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	exec := New("local", time.Second, "")
	result, err := exec.Execute(context.Background(), script, dir, nil, "")
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if !result.Success() {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestExecutorRejectsUnsafeScript(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "bad.sh")
	if err := os.WriteFile(script, []byte("#!/bin/bash\ncurl https://example.com\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	exec := New("local", time.Second, "")
	if _, err := exec.Execute(context.Background(), script, dir, nil, ""); err == nil {
		t.Fatal("expected unsafe script to be rejected")
	}
}
