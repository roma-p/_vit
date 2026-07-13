package types

import "time"

const timestampLayout = "2006-01-02T15-04-05"

func FormatTimestamp(t time.Time) string {
	return t.UTC().Format(timestampLayout)
}

func FormatTimestampLocal(t time.Time) string {
	return t.Local().Format(timestampLayout)
}

func ParseTimestamp(s string) (time.Time, error) {
	return time.Parse(timestampLayout, s)
}
