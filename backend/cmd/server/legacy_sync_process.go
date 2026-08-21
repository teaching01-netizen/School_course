package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type legacySyncProcess struct {
	cancel  context.CancelFunc
	done    chan struct{}
	waitErr error
}

func resolveLegacySyncBinary(configuredPath, serverPath string, lookPath func(string) (string, error)) (string, bool, error) {
	if configuredPath != "" {
		info, err := os.Stat(configuredPath)
		if err != nil {
			return "", false, fmt.Errorf("stat configured legacy sync worker %q: %w", configuredPath, err)
		}
		if info.IsDir() {
			return "", false, fmt.Errorf("configured legacy sync worker %q is a directory", configuredPath)
		}
		return configuredPath, true, nil
	}

	if serverPath != "" {
		siblingPath := filepath.Join(filepath.Dir(serverPath), "legacy-sync")
		if info, err := os.Stat(siblingPath); err == nil && !info.IsDir() {
			return siblingPath, true, nil
		}
	}

	if lookPath == nil {
		lookPath = exec.LookPath
	}
	if path, err := lookPath("legacy-sync"); err == nil {
		return path, true, nil
	}

	return "", false, nil
}

func startLegacySyncProcess(parent context.Context, log *slog.Logger) (*legacySyncProcess, error) {
	serverPath, _ := os.Executable()
	workerPath, found, err := resolveLegacySyncBinary(
		strings.TrimSpace(os.Getenv("LEGACY_SYNC_BINARY")),
		serverPath,
		exec.LookPath,
	)
	if err != nil {
		return nil, err
	}
	if !found {
		log.Warn("legacy sync worker not started: binary not found; queued legacy jobs require a built worker or LEGACY_SYNC_BINARY")
		return nil, nil
	}

	workerCtx, cancel := context.WithCancel(parent)
	cmd := exec.CommandContext(workerCtx, workerPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		cancel()
		return nil, fmt.Errorf("start legacy sync worker %q: %w", workerPath, err)
	}

	worker := &legacySyncProcess{
		cancel: cancel,
		done:   make(chan struct{}),
	}
	go func() {
		worker.waitErr = cmd.Wait()
		close(worker.done)
	}()

	log.Info("legacy sync worker started", "binary", workerPath, "pid", cmd.Process.Pid)
	return worker, nil
}

func (p *legacySyncProcess) Wait(ctx context.Context) error {
	if p == nil {
		return nil
	}
	select {
	case <-p.done:
		return p.waitErr
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (p *legacySyncProcess) Stop() {
	if p != nil {
		p.cancel()
	}
}

func monitorLegacySyncProcess(ctx context.Context, process *legacySyncProcess, log *slog.Logger) {
	err := process.Wait(context.Background())
	if ctx.Err() != nil {
		return
	}
	if err != nil {
		log.Error("legacy sync worker stopped unexpectedly", "err", err)
	} else {
		log.Error("legacy sync worker exited unexpectedly")
	}
	os.Exit(1)
}
