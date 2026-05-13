package sandbox

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestNewDockerSandbox(t *testing.T) {
	sb, err := New(context.Background())
	if err != nil {
		t.Skipf("Docker 不可用: %v", err)
	}
	defer sb.Close()
}

func TestRunCommand(t *testing.T) {
	sb, err := New(context.Background())
	if err != nil {
		t.Skipf("Docker 不可用: %v", err)
	}
	defer sb.Close()

	tmp := t.TempDir()
	f := filepath.Join(tmp, "hello.sh")
	if err := os.WriteFile(f, []byte("#!/bin/sh\necho hello-world"), 0755); err != nil {
		t.Fatal(err)
	}

	cfg := Config{
		Image:           "alpine:latest",
		WorkspaceDir:    tmp,
		NetworkDisabled: true,
		TimeoutSeconds:  30,
	}

	out, err := sb.RunCommand(context.Background(), cfg, "sh /workspace/hello.sh")
	if err != nil {
		t.Fatalf("RunCommand failed: %v", err)
	}

	if out.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d", out.ExitCode)
	}
	if out.Stdout != "hello-world\n" {
		t.Errorf("expected 'hello-world\\n', got %q", out.Stdout)
	}
}

func TestRunCommandFailure(t *testing.T) {
	sb, err := New(context.Background())
	if err != nil {
		t.Skipf("Docker 不可用: %v", err)
	}
	defer sb.Close()

	tmp := t.TempDir()
	cfg := Config{
		Image:           "alpine:latest",
		WorkspaceDir:    tmp,
		NetworkDisabled: true,
		TimeoutSeconds:  30,
	}

	out, err := sb.RunCommand(context.Background(), cfg, "sh -c 'exit 1'")
	if err != nil {
		t.Fatalf("RunCommand returned error: %v", err)
	}

	if out.ExitCode != 1 {
		t.Errorf("expected exit code 1, got %d", out.ExitCode)
	}
}
