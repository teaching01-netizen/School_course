package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

func TestResolveLegacySyncBinaryUsesConfiguredPath(t *testing.T) {
	workerPath := filepath.Join(t.TempDir(), "legacy-sync")
	if err := os.WriteFile(workerPath, []byte("worker"), 0o755); err != nil {
		t.Fatal(err)
	}

	got, found, err := resolveLegacySyncBinary(workerPath, "", func(string) (string, error) {
		return "", errors.New("should not search PATH")
	})
	if err != nil {
		t.Fatalf("resolveLegacySyncBinary returned error: %v", err)
	}
	if !found || got != workerPath {
		t.Fatalf("resolveLegacySyncBinary = (%q, %t), want (%q, true)", got, found, workerPath)
	}
}

func TestResolveLegacySyncBinaryUsesServerSibling(t *testing.T) {
	dir := t.TempDir()
	serverPath := filepath.Join(dir, "server")
	workerPath := filepath.Join(dir, "legacy-sync")
	if err := os.WriteFile(workerPath, []byte("worker"), 0o755); err != nil {
		t.Fatal(err)
	}

	got, found, err := resolveLegacySyncBinary("", serverPath, func(string) (string, error) {
		return "", errors.New("not found")
	})
	if err != nil {
		t.Fatalf("resolveLegacySyncBinary returned error: %v", err)
	}
	if !found || got != workerPath {
		t.Fatalf("resolveLegacySyncBinary = (%q, %t), want (%q, true)", got, found, workerPath)
	}
}

func TestResolveLegacySyncBinaryAllowsLocalServerWithoutWorker(t *testing.T) {
	got, found, err := resolveLegacySyncBinary("", filepath.Join(t.TempDir(), "server"), func(string) (string, error) {
		return "", errors.New("not found")
	})
	if err != nil {
		t.Fatalf("resolveLegacySyncBinary returned error: %v", err)
	}
	if found || got != "" {
		t.Fatalf("resolveLegacySyncBinary = (%q, %t), want (\"\", false)", got, found)
	}
}

func TestResolveLegacySyncBinaryRejectsMissingConfiguredPath(t *testing.T) {
	_, found, err := resolveLegacySyncBinary(filepath.Join(t.TempDir(), "missing-worker"), "", func(string) (string, error) {
		return "", errors.New("should not search PATH")
	})
	if err == nil {
		t.Fatal("resolveLegacySyncBinary returned nil error for missing configured worker")
	}
	if found {
		t.Fatal("resolveLegacySyncBinary reported a missing worker as found")
	}
}

func TestStartLegacySyncProcessRunsConfiguredWorker(t *testing.T) {
	workerPath := filepath.Join(t.TempDir(), "legacy-sync")
	if err := os.WriteFile(workerPath, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LEGACY_SYNC_BINARY", workerPath)

	worker, err := startLegacySyncProcess(context.Background(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("startLegacySyncProcess returned error: %v", err)
	}
	if worker == nil {
		t.Fatal("startLegacySyncProcess returned no worker")
	}
	if err := worker.Wait(context.Background()); err != nil {
		t.Fatalf("managed worker exited with error: %v", err)
	}
	worker.Stop()
}
