package scheduleconflictshttp

import (
	"strings"
	"testing"
)

func TestDefaultPageQueryLimitsCanonicalKeysBeforeEnrichment(t *testing.T) {
	// Given: the common unfiltered first-page request.
	query, args := pageQuery(listFilters{Limit: 50})

	// When: its specialized SQL is inspected.
	pageAt := strings.Index(query, "page_keys AS")
	enrichmentAt := strings.LastIndex(query, "JOIN courses pc")

	// Then: canonical pair keys are limited before metadata and JSON work.
	if pageAt < 0 || enrichmentAt < 0 || pageAt > enrichmentAt || !strings.Contains(query[pageAt:enrichmentAt], "LIMIT $1") {
		t.Fatalf("default query does not limit keys before enrichment:\n%s", query)
	}
	if len(args) != 1 || args[0] != 51 {
		t.Fatalf("args = %#v", args)
	}
}

func TestPageQueryEmitsCanonicalPairsWithoutDistinct(t *testing.T) {
	// Given: the default pair-discovery query.
	query, _ := pageQuery(listFilters{Limit: 50})

	// When: both session and student overlap branches are considered.
	// Then: overridden peers have one deterministic orientation and no DISTINCT cleanup pass.
	for _, fragment := range []string{
		"NOT (s2.conflict_override OR s2.legacy_conflict_override) OR s1.id < s2.id",
		"NOT b2.conflict_override OR b1.session_id < b2.session_id",
	} {
		if !strings.Contains(query, fragment) {
			t.Fatalf("query missing canonical emission guard %q", fragment)
		}
	}
	if strings.Contains(query, "SELECT DISTINCT") {
		t.Fatal("query must not generate symmetric pairs and deduplicate them")
	}
}

func TestSummaryQueryCountsKeysWithoutEnrichment(t *testing.T) {
	// Given: an unfiltered summary request.
	query, args := summaryQuery(listFilters{})

	// When: its query plan shape is inspected.
	// Then: it counts canonical keys without course, subject, room, user, or JSON enrichment.
	for _, forbidden := range []string{"JOIN courses", "JOIN subjects", "jsonb_", "numbered AS MATERIALIZED"} {
		if strings.Contains(query, forbidden) {
			t.Fatalf("summary query unexpectedly contains %q", forbidden)
		}
	}
	if len(args) != 0 || !strings.Contains(query, "FROM pairs") {
		t.Fatalf("unexpected summary query or args: %#v\n%s", args, query)
	}
}

func TestFilteredPageQueryUsesTypedParameters(t *testing.T) {
	// Given: any filtered request.
	query, _ := pageQuery(listFilters{Limit: 50, ConflictType: "teacher_overlap"})

	// When: its predicates are inspected.
	// Then: UUIDs remain UUIDs and empty-string OR predicates are absent.
	if strings.Contains(query, "teacher_id::text") || strings.Contains(query, "$3 = ''") || !strings.Contains(query, "$3::uuid") {
		t.Fatalf("filtered query does not preserve typed parameters:\n%s", query)
	}
}
