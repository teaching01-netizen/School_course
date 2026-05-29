package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"time"

	"warwick-institute/internal/crmclient"
)

func main() {
	var (
		baseURL      = flag.String("base-url", "", "CRM base URL (or CRM_BASE_URL env)")
		username     = flag.String("username", "", "CRM username (or CRM_USERNAME env)")
		password     = flag.String("password", "", "CRM password (or CRM_PASSWORD env)")
		output       = flag.String("output", "", "save XLSX to file path (optional)")
		criteria     = flag.String("criteria", "Key0=41&Key4=35&Key41=864&Key62=1012", "report criteria query params")
		stepLogin    = flag.Bool("step-login", false, "only test login, don't download")
		loginTimeout = flag.Duration("login-timeout", 5*time.Minute, "overall login timeout (includes lock/continue retries)")
	)
	flag.Parse()

	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))

	// Resolve credentials: flag > env > prompt user.
	resolve := func(flagVal, envName string) string {
		if flagVal != "" {
			return flagVal
		}
		return os.Getenv(envName)
	}

	u := resolve(*baseURL, "CRM_BASE_URL")
	pUser := resolve(*username, "CRM_USERNAME")
	pPass := resolve(*password, "CRM_PASSWORD")

	if u == "" {
		u = "http://warwickins.sundaehost.com/crm"
	}
	if pUser == "" || pPass == "" {
		fmt.Fprintln(os.Stderr, "ERROR: CRM_USERNAME and CRM_PASSWORD environment variables are required")
		os.Exit(1)
	}

	fmt.Println("=== CRM Client Test ===")
	fmt.Printf("Base URL: %s\n", u)
	fmt.Printf("Username: %s\n", pUser)
	fmt.Printf("Password: %s\n", "******")
	fmt.Println()

	// Parse criteria.
	criteriaVals, err := url.ParseQuery(*criteria)
	if err != nil {
		log.Error("parse criteria", "err", err)
		os.Exit(1)
	}

	client, err := crmclient.New(crmclient.Config{
		BaseURL:         u,
		Username:        pUser,
		Password:        pPass,
		RequestTimeout:  30 * time.Second,
		DownloadTimeout: 120 * time.Second,
	})
	if err != nil {
		log.Error("create client", "err", err)
		os.Exit(1)
	}

	// Step 1: Login.
	fmt.Print("Logging in... ")
	ctx, cancel := context.WithTimeout(context.Background(), *loginTimeout)
	defer cancel()

	loginStart := time.Now()
	if err := client.Login(ctx); err != nil {
		fmt.Println("FAIL")
		log.Error("login failed", "err", err)
		os.Exit(1)
	}
	fmt.Printf("OK (SID=%s, took=%v)\n", client.SID(), time.Since(loginStart).Round(time.Millisecond))

	if *stepLogin {
		fmt.Println("\nStep-login mode: skipping download.")
		return
	}

	// Step 2: Download XLSX.
	fmt.Println("Downloading XLSX...")
	dlCtx, dlCancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer dlCancel()

	dlStart := time.Now()
	data, err := client.DownloadXLSX(dlCtx, criteriaVals)
	if err != nil {
		fmt.Println("FAIL")
		log.Error("download failed", "err", err)
		os.Exit(1)
	}
	fmt.Printf("OK (%d bytes, took=%v)\n", len(data), time.Since(dlStart).Round(time.Millisecond))

	// Show some info about the XLSX.
	fmt.Println()
	fmt.Printf("XLSX size: %d bytes\n", len(data))
	if len(data) > 0 {
		// XLSX files are ZIP archives. Check the magic bytes.
		if len(data) >= 4 && data[0] == 0x50 && data[1] == 0x4B && data[2] == 0x03 && data[3] == 0x04 {
			fmt.Println("Format: valid ZIP/XLSX (PK\x03\x04 header)")
		} else {
			fmt.Printf("Warning: unexpected first bytes: %x\n", data[:min(len(data), 16)])
		}
	}

	// Save to file if requested.
	if *output != "" {
		if err := os.WriteFile(*output, data, 0644); err != nil {
			log.Error("save output", "err", err)
			os.Exit(1)
		}
		fmt.Printf("Saved to: %s\n", *output)
	} else {
		// Default: write to a temp file so the user can inspect it.
		tmpFile := fmt.Sprintf("/tmp/crm-export-%d.xlsx", time.Now().Unix())
		if err := os.WriteFile(tmpFile, data, 0644); err != nil {
			log.Error("save temp file", "err", err)
			os.Exit(1)
		}
		fmt.Printf("Saved to: %s\n", tmpFile)
	}

	fmt.Println("\n=== Test complete ===")
}
