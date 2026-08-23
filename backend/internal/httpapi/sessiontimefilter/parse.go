package sessiontimefilter

import (
	"fmt"
	"net/url"
	"strings"
	"time"
)

func Parse(query url.Values) (string, string, error) {
	from := strings.TrimSpace(query.Get("session_from"))
	to := strings.TrimSpace(query.Get("session_to"))
	for _, bound := range []struct {
		name  string
		value string
	}{
		{name: "session_from", value: from},
		{name: "session_to", value: to},
	} {
		if bound.value == "" {
			continue
		}
		if _, err := time.Parse("15:04", bound.value); err != nil {
			return "", "", fmt.Errorf("%s must be a valid 24-hour time", bound.name)
		}
	}
	if from != "" && to != "" && from > to {
		return "", "", fmt.Errorf("session_from must be earlier than or equal to session_to")
	}
	return from, to, nil
}
