package model

import "time"

// The plugin stores all timestamps in a single canonical format so the value
// written by one component can be read and compared by another (e.g. the AI
// prompt context vs. CreatedAt/FiredAt fields). Currently that format is Unix
// milliseconds; a future change may migrate storage to RFC 3339 strings, at
// which point the signatures below will change and all call sites will be
// updated to match.

// NowTimestamp returns the current time in the canonical storage format.
func NowTimestamp() int64 {
	return time.Now().UnixMilli()
}

// TimeToTimestamp converts a time.Time to the canonical storage format.
func TimeToTimestamp(t time.Time) int64 {
	return t.UnixMilli()
}

// TimestampToTime converts a canonical storage timestamp back to time.Time.
func TimestampToTime(ts int64) time.Time {
	return time.UnixMilli(ts)
}
