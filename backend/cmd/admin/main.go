package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"time"

	"warwick-institute/internal/config"
	sqldb "warwick-institute/internal/db"
	"warwick-institute/internal/logging"
	"warwick-institute/internal/pg"
	"warwick-institute/internal/users"

	"github.com/google/uuid"
)

func main() {
	var (
		username = flag.String("username", "", "username (required)")
		role     = flag.String("role", "Admin", "Admin|Teacher")
		password = flag.String("password", "", "password (required)")
	)
	flag.Parse()

	cfg, err := config.FromEnv()
	if err != nil {
		slog.New(slog.NewTextHandler(os.Stderr, nil)).Error("config error", "err", err)
		os.Exit(2)
	}

	log := logging.New(cfg.LogLevel)

	if *username == "" || *password == "" {
		log.Error("missing required flags: -username and -password")
		os.Exit(2)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	dbpool, err := pg.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Error("db connect", "err", err)
		os.Exit(1)
	}
	defer dbpool.Close()

	q := sqldb.New(dbpool)
	adminSvc := users.NewAdminProvisioningService(
		users.SQLCAdminUserStore{Q: q},
		users.AuthPasswordHasher{Pepper: cfg.AuthPepper},
	)
	id, err := adminSvc.ProvisionUser(ctx, users.Actor{ID: uuid.New(), Role: users.RoleAdmin}, *username, *role, *password)
	if err != nil {
		log.Error("create user", "err", err)
		os.Exit(1)
	}

	fmt.Println(id.String())
}
