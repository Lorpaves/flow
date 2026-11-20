package core

import (
	"errors"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var relativeDate = regexp.MustCompile(`^(\d+)\s*(h|hours?|d|days?|w|weeks?|m|months?|小时|天|周|个月)$`)

// ParseDate accepts calendar dates and relative expressions. Relative dates use
// the user's local calendar, avoiding DST shifts from 24-hour arithmetic.
func ParseDate(input string, now time.Time) (string, error) {
	s := strings.ToLower(strings.TrimSpace(input))
	if s == "" {
		return "", nil
	}
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	var date time.Time
	switch s {
	case "today", "今天":
		date = today
	case "tomorrow", "明天":
		date = today.AddDate(0, 0, 1)
	case "后天":
		date = today.AddDate(0, 0, 2)
	case "end of this week", "this weekend", "本周末":
		date = today.AddDate(0, 0, (7-int(today.Weekday()))%7)
	case "next week", "in one week", "下周", "一周后":
		date = today.AddDate(0, 0, 7)
	default:
		if m := relativeDate.FindStringSubmatch(s); m != nil {
			n, e := strconv.Atoi(m[1])
			if e != nil || n > 36500 {
				return "", errors.New("invalid_date")
			}
			switch m[2][0] {
			case 'h':
				date = now.Add(time.Duration(n) * time.Hour)
			case 'w':
				date = today.AddDate(0, 0, n*7)
			case 'm':
				date = today.AddDate(0, n, 0)
			default:
				switch m[2] {
				case "小时":
					date = now.Add(time.Duration(n) * time.Hour)
				case "周":
					date = today.AddDate(0, 0, n*7)
				case "个月":
					date = today.AddDate(0, n, 0)
				default:
					date = today.AddDate(0, 0, n)
				}
			}
		} else {
			for _, layout := range []string{"2006-01-02", "2006/1/2", "2 Jan 2006", "Jan 2 2006", "January 2 2006", "Jan 2", "2 Jan", "January 2"} {
				candidate, err := time.ParseInLocation(layout, s, now.Location())
				if err != nil {
					continue
				}
				if candidate.Year() == 0 {
					candidate = time.Date(now.Year(), candidate.Month(), candidate.Day(), 0, 0, 0, 0, now.Location())
					if candidate.Before(today) {
						candidate = candidate.AddDate(1, 0, 0)
					}
				}
				date = candidate
				break
			}
		}
	}
	if date.IsZero() || date.Year() > 9999 {
		return "", errors.New("invalid_date")
	}
	return date.Format("2006-01-02"), nil
}
