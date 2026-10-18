// Package dbms implements the minimal in-memory SQL subset from spec/sql.md.
package dbms

import "fmt"

// Type is an SQL column or literal type.
type Type uint8

const (
	Integer Type = iota
	Real
	Text
)

// Value preserves the literal type. Its fields are private to prevent invalid values.
type Value struct {
	typeID  Type
	integer int64
	real    float64
	text    string
}

func (v Value) Type() Type             { return v.typeID }
func (v Value) Integer() (int64, bool) { return v.integer, v.typeID == Integer }
func (v Value) Real() (float64, bool)  { return v.real, v.typeID == Real }
func (v Value) Text() (string, bool)   { return v.text, v.typeID == Text }

// Result owns copies of selected values; changing it cannot change stored data.
type Result struct {
	Columns []string
	Rows    [][]Value
}

// QueryError uses a zero-based byte offset relative to one input statement.
type QueryError struct {
	Message  string
	Position int
}

func (e *QueryError) Error() string { return e.Message }
func queryError(position int, format string, args ...any) error {
	return &QueryError{Message: fmt.Sprintf(format, args...), Position: position}
}
