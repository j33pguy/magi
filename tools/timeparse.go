package tools

import (
	"fmt"
	"regexp"
	"strconv"
	"time"
)

var relativePattern = regexp.MustCompile(`^(\d+)([dwmyDWMY])$`)

// parseTimeParam parses a time string that is either a relative duration
// ("7d", "2w", "1m", "1y") or an absolute date ("2006-01-02" or RFC3339).
func ParseTimeParam(s string) (*time.Time, error) {
	if s == "" {
		return nil, nil
	}

	// Try relative: "7d", "2w", "1m", "1y"
	if matched := relativePattern.FindStringSubmatch(s); matched != nil {
		n, _ := strconv.Atoi(matched[1])
		now := time.Now().UTC()
		var t time.Time
		switch matched[2] {
		case "d", "D":
			t = now.AddDate(0, 0, -n)
		case "w", "W":
			t = now.AddDate(0, 0, -n*7)
		case "m", "M":
			t = now.AddDate(0, -n, 0)
		case "y", "Y":
			t = now.AddDate(-n, 0, 0)
		}
		return &t, nil
	}

	// Try RFC3339: "2006-01-02T15:04:05Z"
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return &t, nil
	}

	// Try date only: "2006-01-02"
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return &t, nil
	}

	return nil, fmt.Errorf("invalid time format %q: use relative (7d, 2w, 1m, 1y) or absolute (2006-01-02, RFC3339)", s)
}
