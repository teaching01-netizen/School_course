package sessiondaterange

import (
	"fmt"
	"net/url"
	"strings"
	"time"
)

func Parse(query url.Values) (string, string, error) {
	from := strings.TrimSpace(query.Get("session_date_from"))
	to := strings.TrimSpace(query.Get("session_date_to"))
	for _, bound := range []struct {
		name  string
		value string
	}{
		{name: "session_date_from", value: from},
		{name: "session_date_to", value: to},
	} {
		if bound.value == "" {
			continue
		}
		if _, err := time.Parse("2006-01-02", bound.value); err != nil {
			return "", "", fmt.Errorf("%s must be a valid calendar date", bound.name)
		}
	}
	if from != "" && to != "" {
		fromDate, fromErr := time.Parse("2006-01-02", from)
		toDate, toErr := time.Parse("2006-01-02", to)
		if fromErr != nil || toErr != nil {
			return "", "", fmt.Errorf("session_date_from and session_date_to must be valid calendar dates")
		}
		if fromDate.After(toDate) {
			return "", "", fmt.Errorf("session_date_from must be earlier than or equal to session_date_to")
		}
	}
	return from, to, nil
}
