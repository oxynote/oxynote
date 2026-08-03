package timeutil

import (
	"database/sql/driver"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Term holds the term data.
type Term struct {
	// Year specifies the year.
	Year int `json:"year" db:"-"`

	// Month specifies the month.
	Month time.Month `json:"month" db:"-"`
}

// Equal checks if the term is equal to another term.
func (t Term) Equal(ot Term) bool {
	return t.Year == ot.Year && t.Month == ot.Month
}

// Scan converts the database column data into a Term struct.
func (t *Term) Scan(src any) error {
	var raw string

	switch v := src.(type) {
	case string:
		raw = v
	case []byte:
		raw = string(v)
	default:
		return fmt.Errorf("cannot convert %T to term", src)
	}

	parts := strings.Split(raw, "_")
	if len(parts) != 2 {
		return fmt.Errorf("invalid term format: %s", raw)
	}

	year, err := strconv.Atoi(parts[0])
	if err != nil {
		return fmt.Errorf("invalid term year: %s", parts[0])
	}

	month, err := strconv.Atoi(parts[1])
	if err != nil {
		return fmt.Errorf("invalid term month: %s", parts[1])
	}

	*t = Term{
		Year:  year,
		Month: time.Month(month),
	}

	return nil
}

// Value converts the Term struct into a database column data.
func (t Term) Value() (driver.Value, error) {
	return fmt.Sprintf("%d_%d", t.Year, t.Month), nil
}
