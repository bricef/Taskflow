package sqlite

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"
)

// SQLiteBool handles SQLite INTEGER (0/1) ↔ Go bool conversion.
type SQLiteBool bool

func (b *SQLiteBool) Scan(src any) error {
	switch v := src.(type) {
	case int64:
		*b = v != 0
	case bool:
		*b = SQLiteBool(v)
	case nil:
		*b = false
	default:
		return fmt.Errorf("SQLiteBool.Scan: unsupported type %T", src)
	}
	return nil
}

func (b SQLiteBool) Value() (driver.Value, error) {
	if b {
		return int64(1), nil
	}
	return int64(0), nil
}

// Timestamp handles SQLite TEXT timestamp ↔ Go time.Time conversion.
// Supports reading multiple formats (RFC3339, millisecond precision, SQLite datetime).
type Timestamp time.Time

func (t *Timestamp) Scan(src any) error {
	switch v := src.(type) {
	case string:
		parsed, err := parseTimeString(v)
		if err != nil {
			return err
		}
		*t = Timestamp(parsed)
	case nil:
		*t = Timestamp(time.Time{})
	default:
		return fmt.Errorf("Timestamp.Scan: unsupported type %T", src)
	}
	return nil
}

func (t Timestamp) Value() (driver.Value, error) {
	return time.Time(t).Format(tsFormat), nil
}

func (t Timestamp) Time() time.Time {
	return time.Time(t)
}

// NullTimestamp handles nullable SQLite TEXT timestamp ↔ Go *time.Time.
type NullTimestamp struct {
	Time  time.Time
	Valid bool
}

func (t *NullTimestamp) Scan(src any) error {
	if src == nil {
		t.Valid = false
		return nil
	}
	s, ok := src.(string)
	if !ok {
		return fmt.Errorf("NullTimestamp.Scan: unsupported type %T", src)
	}
	parsed, err := parseTimeString(s)
	if err != nil {
		return err
	}
	t.Time = parsed
	t.Valid = true
	return nil
}

func (t NullTimestamp) Value() (driver.Value, error) {
	if !t.Valid {
		return nil, nil
	}
	return t.Time.Format(tsFormat), nil
}

func (t NullTimestamp) TimePtr() *time.Time {
	if !t.Valid {
		return nil
	}
	return &t.Time
}

// StringList handles SQLite JSON TEXT ↔ Go []string conversion.
type StringList []string

func (s *StringList) Scan(src any) error {
	switch v := src.(type) {
	case string:
		return json.Unmarshal([]byte(v), s)
	case []byte:
		return json.Unmarshal(v, s)
	case nil:
		*s = []string{}
	default:
		return fmt.Errorf("StringList.Scan: unsupported type %T", src)
	}
	if *s == nil {
		*s = []string{}
	}
	return nil
}

func (s StringList) Value() (driver.Value, error) {
	if s == nil {
		return "[]", nil
	}
	b, err := json.Marshal([]string(s))
	return string(b), err
}

// JSONRaw handles SQLite TEXT ↔ json.RawMessage.
// json.RawMessage's built-in Scanner only handles []byte, not string.
type JSONRaw json.RawMessage

func (j *JSONRaw) Scan(src any) error {
	switch v := src.(type) {
	case string:
		*j = JSONRaw(v)
	case []byte:
		*j = JSONRaw(v)
	case nil:
		*j = JSONRaw(`{}`)
	default:
		return fmt.Errorf("JSONRaw.Scan: unsupported type %T", src)
	}
	return nil
}

func (j JSONRaw) Value() (driver.Value, error) {
	if j == nil {
		return "{}", nil
	}
	return string(j), nil
}

// Timestamp format matching the SQLite schema defaults.
const tsFormat = "2006-01-02T15:04:05.000Z"

func parseTimeString(s string) (time.Time, error) {
	for _, layout := range []string{
		"2006-01-02T15:04:05.000Z",
		time.RFC3339,
		"2006-01-02 15:04:05",
	} {
		t, err := time.Parse(layout, s)
		if err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("cannot parse time %q", s)
}
