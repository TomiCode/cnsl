package store

import (
	"database/sql/driver"
	"fmt"
	"time"
)

const timeLayout = "2006-01-02T15:04:05.000Z07:00"

// Time is a timestamp stored as RFC 3339 UTC text (the schema's convention).
type Time struct {
	time.Time
}

func Now() Time {
	return Time{time.Now().UTC()}
}

func (t Time) Value() (driver.Value, error) {
	return t.UTC().Format(timeLayout), nil
}

func (t *Time) Scan(src any) error {
	switch v := src.(type) {
	case string:
		return t.parse(v)
	case []byte:
		return t.parse(string(v))
	case time.Time:
		t.Time = v.UTC()
		return nil
	default:
		return fmt.Errorf("store.Time: cannot scan %T", src)
	}
}

func (t *Time) parse(s string) error {
	parsed, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return fmt.Errorf("store.Time: %w", err)
	}
	t.Time = parsed.UTC()
	return nil
}

// NullTime is a nullable Time.
type NullTime struct {
	Time  Time
	Valid bool
}

func (n NullTime) Value() (driver.Value, error) {
	if !n.Valid {
		return nil, nil
	}
	return n.Time.Value()
}

func (n *NullTime) Scan(src any) error {
	if src == nil {
		n.Valid = false
		return nil
	}
	n.Valid = true
	return n.Time.Scan(src)
}

// Ptr returns the time as a pointer, nil when not valid.
func (n NullTime) Ptr() *time.Time {
	if !n.Valid {
		return nil
	}
	t := n.Time.Time
	return &t
}

func NewNullTime(t time.Time) NullTime {
	return NullTime{Time: Time{t.UTC()}, Valid: true}
}
