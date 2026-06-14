package emailnotifier

import (
	"strings"
	"testing"
)

func TestBuildSitInTableEscapesCellContent(t *testing.T) {
	table := BuildSitInTable([]SitInReminderData{{
		StudentNickname:    `Nick & Co`,
		WCode:              `W250&lt;1`,
		CourseName:         `Math <Advanced>`,
		SitInCourseName:    `SAT & Physics&gt;`,
		SitInDate:          `Mon 1 Jan 2026`,
		SitInTime:          `08:00 - 09:30`,
		MissedSessionsInfo: `Missed & Info&gt;`,
	}})

	if strings.Contains(table, `<script>`) {
		t.Fatalf("table contains raw script tag: %s", table)
	}
	for _, want := range []string{
		`Nick &amp; Co`,
		`W250&amp;lt;1`,
		`Math &lt;Advanced&gt;`,
		`SAT &amp; Physics&amp;gt;`,
		`Missed &amp; Info&amp;gt;`,
	} {
		if !strings.Contains(table, want) {
			t.Fatalf("table missing escaped content %q: %s", want, table)
		}
	}
}
