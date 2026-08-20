// capture-students is a one-off development tool: it logs into the legacy
// site with the configured LEGACY_SYNC_* credentials, fetches the
// /Admin/Students page, submits the search form (the listing is empty until
// searched), and saves both pages to files so their shapes can be turned into
// parser fixtures. Read-only GET + the same search POST the site UI performs;
// never commits anything.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"

	"warwick-institute/internal/legacysync"
)

func main() {
	plainOut := flag.String("out", "students_page.html", "plain page output file")
	searchOut := flag.String("search-out", "students_page_search.html", "searched page output file")
	searchText := flag.String("search", "", "SearchText to submit (empty = all students)")
	flag.Parse()
	baseURL := os.Getenv("LEGACY_SYNC_URL")
	username := os.Getenv("LEGACY_SYNC_USERNAME")
	password := os.Getenv("LEGACY_SYNC_PASSWORD")
	if baseURL == "" || username == "" || password == "" {
		log.Fatal("LEGACY_SYNC_URL / LEGACY_SYNC_USERNAME / LEGACY_SYNC_PASSWORD are required")
	}
	client, err := legacysync.NewClient(baseURL, username, password)
	if err != nil {
		log.Fatalf("client: %v", err)
	}
	ctx := context.Background()
	if err := client.Login(); err != nil {
		log.Fatalf("login: %v", err)
	}
	page, err := client.FetchStudentsPageContext(ctx)
	if err != nil {
		log.Fatalf("fetch students page: %v", err)
	}
	if err := os.WriteFile(*plainOut, []byte(page), 0o644); err != nil {
		log.Fatalf("write: %v", err)
	}
	fmt.Printf("wrote %d bytes to %s\n", len(page), *plainOut)

	searched, err := client.SearchStudentsPageContext(ctx, *searchText)
	if err != nil {
		log.Fatalf("search students page: %v", err)
	}
	if err := os.WriteFile(*searchOut, []byte(searched), 0o644); err != nil {
		log.Fatalf("write: %v", err)
	}
	fmt.Printf("wrote %d bytes to %s\n", len(searched), *searchOut)

	// The page after the search includes the antiforgery token the next
	// request needs; a follow-up GET on the searched page would re-render the
	// same table, so also fetch once more and report whether it changed.
	again, err := client.FetchStudentsPageContext(ctx)
	if err != nil {
		log.Fatalf("fetch students page again: %v", err)
	}
	fmt.Printf("plain page after search: %d bytes\n", len(again))
}

var _ = http.MethodPost
var _ = url.Values{}
