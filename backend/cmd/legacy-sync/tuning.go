package main

import (
	"os"
	"strconv"
)

// workerConcurrency resolves LEGACY_SYNC_WORKERS, falling back to the client's
// in-flight ceiling when the env var is unset or invalid (intEnv already treats
// 0 and junk as fallback; a worker count of 0 is meaningless).
func workerConcurrency(clientMax int) int { return intEnv("LEGACY_SYNC_WORKERS", clientMax) }

// reconcileWorkers resolves LEGACY_SYNC_RECONCILE_WORKERS with a zero-preserving
// parse so 0 selects the exact serial path. Unset/junk -> default; 0/1 -> serial.
func reconcileWorkers(clientMax int) int {
	raw := os.Getenv("LEGACY_SYNC_RECONCILE_WORKERS")
	if raw == "" {
		return min(clientMax, 16)
	}
	if parsed, err := strconv.Atoi(raw); err == nil && parsed >= 0 {
		return parsed
	}
	return min(clientMax, 16)
}

// maxPoolConns returns the pgx pool connection budget.
func maxPoolConns(env, workers int) int {
	if env > 0 {
		return env
	}
	if pool := 2 * workers; pool > 64 {
		return pool
	}
	return 64
}
