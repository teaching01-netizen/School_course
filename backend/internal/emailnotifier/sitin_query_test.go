package emailnotifier

import (
	"strings"
	"testing"
)

func TestBuildSitInTableEscapesCellContent(t *testing.T) {
	table := BuildSitInTable([]SitInReminderData{{
		StudentName:     `<script>alert("x")</script>`,
		StudentNickname: `Nick & Co`,
		CourseName:      `Math <Advanced>`,
		CourseCode:      `SAT&1`,
	}})

	if strings.Contains(table, `<script>`) {
		t.Fatalf("table contains raw script tag: %s", table)
	}
	for _, want := range []string{
		`&lt;script&gt;alert(&#34;x&#34;)&lt;/script&gt;`,
		`Nick &amp; Co`,
		`Math &lt;Advanced&gt;`,
		`SAT&amp;1`,
	} {
		if !strings.Contains(table, want) {
			t.Fatalf("table missing escaped content %q: %s", want, table)
		}
	}
}
