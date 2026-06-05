package main

import (
	"fmt"
	"os"
	"strings"

	"warwick-institute/internal/legacysync"
)

func main() {
	baseURL := envOr("LEGACY_SYNC_URL", "https://warwick.azurewebsites.net")
	username := os.Getenv("LEGACY_SYNC_USERNAME")
	password := os.Getenv("LEGACY_SYNC_PASSWORD")
	courseID := envOr("LEGACY_SYNC_TEST_COURSE_ID", "7090")

	if username == "" || password == "" {
		fmt.Fprintln(os.Stderr, "LEGACY_SYNC_USERNAME and LEGACY_SYNC_PASSWORD required")
		os.Exit(1)
	}

	client, err := legacysync.NewClient(baseURL, username, password)
	if err != nil {
		fmt.Fprintf(os.Stderr, "NewClient: %v\n", err)
		os.Exit(1)
	}

	if err := client.Login(); err != nil {
		fmt.Fprintf(os.Stderr, "Login: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("Login OK")

	html, err := client.FetchSchedulePage(courseID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FetchSchedulePage: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Fetched %d bytes\n", len(html))

	// Save raw HTML for inspection
	os.WriteFile("/tmp/debug_schedule.html", []byte(html), 0644)
	fmt.Println("Saved /tmp/debug_schedule.html")

	// Check for table with class "table"
	if strings.Contains(html, `<table class="table"`) || strings.Contains(html, `<table class="table">`) {
		fmt.Println("Found table.table")
	} else {
		fmt.Println("WARNING: No <table class=\"table\"> found")
		// Show first table found
		tag := "<table"
		idx := strings.Index(html, tag)
		if idx >= 0 {
			end := idx + 400
			if end > len(html) {
				end = len(html)
			}
			fmt.Printf("First <table around byte %d: ...%s...\n", idx, html[idx:end])
		}
	}

	// Check for Date header
	if strings.Contains(html, ">Date<") || strings.Contains(html, ">Date</th") {
		fmt.Println("Found Date header")
	} else {
		fmt.Println("WARNING: No Date header found")
	}

	// Try finding any table
	tableIdx := strings.Index(html, "<table")
	if tableIdx >= 0 {
		fmt.Printf("First <table at byte %d\n", tableIdx)
		// Check if there's a tbody
		if strings.Contains(html[tableIdx:], "tbody") {
			fmt.Println("  Has <tbody>")
		} else {
			fmt.Println("  No <tbody>")
		}
		// Check for <tr> directly under table
		tableEnd := strings.Index(html[tableIdx:], "</table>")
		if tableEnd >= 0 {
			tableContent := html[tableIdx : tableIdx+tableEnd+8]
			trCount := strings.Count(tableContent, "<tr")
			thCount := strings.Count(tableContent, "<th")
			tdCount := strings.Count(tableContent, "<td")
			fmt.Printf("  Table: %d <tr>, %d <th>, %d <td>\n", trCount, thCount, tdCount)
		}
	}

	// Try parsing
	rows, err := legacysync.ParseScheduleTable(html)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ParseScheduleTable: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Parsed %d rows\n", len(rows))
	for i, r := range rows {
		fmt.Printf("  row %d: %s %s-%s %s\n", i, r.Date.Format("2006-01-02"), r.Begin, r.End, r.Classroom)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
