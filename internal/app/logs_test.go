package app

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestTailLogReturnsLastLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "php error.log")
	if err := os.WriteFile(path, []byte("one\ntwo\nthree\n"), 0644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := tailLog(context.Background(), path, 2, false, &out); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != "two\nthree\n" {
		t.Fatalf("unexpected tail %q", got)
	}
}
func TestTailLogMissingIsEmpty(t *testing.T) {
	var out bytes.Buffer
	if err := tailLog(context.Background(), filepath.Join(t.TempDir(), "missing.log"), 100, false, &out); err != nil {
		t.Fatal(err)
	}
	if out.Len() != 0 {
		t.Fatal("expected empty output")
	}
}
func TestLogProjectConfig(t *testing.T) {
	c := parseTOML("version=\"8.4\"\n[logs]\nscope=\"project\"\npath=\".phpvm/errors.log\"\n")
	if c.LogScope != "project" || c.LogPath != ".phpvm/errors.log" {
		t.Fatalf("unexpected logs config: %#v", c)
	}
}
func TestTailLogFollow(t *testing.T) {
	path := filepath.Join(t.TempDir(), "follow.log")
	if err := os.WriteFile(path, nil, 0644); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var out bytes.Buffer
	done := make(chan error, 1)
	go func() { done <- tailLog(ctx, path, 0, true, &out) }()
	time.Sleep(300 * time.Millisecond)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.WriteString("new error\n")
	f.Close()
	time.Sleep(400 * time.Millisecond)
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if out.String() != "new error\n" {
		t.Fatalf("unexpected followed output %q", out.String())
	}
}
