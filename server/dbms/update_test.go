package dbms

import (
	"reflect"
	"strings"
	"testing"
)

func updateSample(t *testing.T) *Database {
	t.Helper()
	db := sample(t)
	execute(t, db, "INSERT INTO sample (id, score, name) VALUES (1, 0.8, 'Alice');")
	execute(t, db, "INSERT INTO sample (id, score, name) VALUES (2, 0.9, 'Bob');")
	execute(t, db, "INSERT INTO sample (id, score, name) VALUES (3, 0.8, 'Alice');")
	return db
}

func TestUpdate(t *testing.T) {
	for _, tc := range []struct {
		name, sql string
		rows      [][]Value
	}{
		{"zero matches", "UPDATE sample SET score = 0.5 WHERE id = 999;", nil},
		{"one match", "UPDATE sample SET score = 0.5 WHERE id = 2;", [][]Value{
			updateRow(1, 0.8, "Alice"), updateRow(2, 0.5, "Bob"), updateRow(3, 0.8, "Alice"),
		}},
		{"many matches", "UPDATE sample SET score = -30.5, name = 'O''Reilly; Привет' WHERE name = 'Alice';", [][]Value{
			updateRow(1, -30.5, "O'Reilly; Привет"), updateRow(2, 0.9, "Bob"), updateRow(3, -30.5, "O'Reilly; Привет"),
		}},
		{"real filter", "UPDATE sample SET name = '' WHERE score = 0.8;", [][]Value{
			updateRow(1, 0.8, ""), updateRow(2, 0.9, "Bob"), updateRow(3, 0.8, ""),
		}},
		{"update filter column", "uPdAtE sample sEt name = 'Bob', id = -30 WhErE name = 'Alice';", [][]Value{
			updateRow(-30, 0.8, "Bob"), updateRow(2, 0.9, "Bob"), updateRow(-30, 0.8, "Bob"),
		}},
		{"same value", "UPDATE sample SET score = 0.8 WHERE score = 0.8;", nil},
		{"integer minimum", "UPDATE sample SET id = -9223372036854775808 WHERE id = 1;", [][]Value{
			updateRow(-9223372036854775808, 0.8, "Alice"), updateRow(2, 0.9, "Bob"), updateRow(3, 0.8, "Alice"),
		}},
		{"integer maximum", "UPDATE sample SET id = 9223372036854775807 WHERE id = 1;", [][]Value{
			updateRow(9223372036854775807, 0.8, "Alice"), updateRow(2, 0.9, "Bob"), updateRow(3, 0.8, "Alice"),
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := updateSample(t)
			before := execute(t, db, "SELECT * FROM sample;")
			result := execute(t, db, tc.sql)
			if len(result.Columns) != 0 || len(result.Rows) != 0 {
				t.Fatalf("UPDATE returned rows: %+v", result)
			}
			want := before
			if tc.rows != nil {
				want.Rows = tc.rows
			}
			got := execute(t, db, "SELECT * FROM sample;")
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("rows = %+v, want %+v", got, want)
			}
			// Previously returned rows must remain a snapshot after UPDATE.
			if !reflect.DeepEqual(before.Rows, [][]Value{updateRow(1, 0.8, "Alice"), updateRow(2, 0.9, "Bob"), updateRow(3, 0.8, "Alice")}) {
				t.Fatal("UPDATE changed a previously returned SELECT result")
			}
		})
	}
	t.Run("empty table", func(t *testing.T) {
		db := sample(t)
		execute(t, db, "UPDATE sample SET name = 'new' WHERE id = 1;")
		if len(execute(t, db, "SELECT * FROM sample;").Rows) != 0 {
			t.Fatal("UPDATE inserted a row")
		}
	})
	t.Run("specification example", func(t *testing.T) {
		db := &Database{}
		execute(t, db, "CREATE TABLE targets (target_id TEXT, latitude REAL, longitude REAL, probability REAL, last_seen_at INTEGER);")
		execute(t, db, "INSERT INTO targets (target_id, latitude, longitude, probability, last_seen_at) VALUES ('t1', 59.95, 30.31, 0.8, 1732000000000);")
		execute(t, db, "UPDATE targets SET probability = 0.9 WHERE target_id = 't1';")
		r := execute(t, db, "SELECT probability, last_seen_at FROM targets WHERE target_id = 't1';")
		if len(r.Rows) != 1 {
			t.Fatalf("wrong result: %+v", r)
		}
		probability, ok := r.Rows[0][0].Real()
		timestamp, integer := r.Rows[0][1].Integer()
		if !ok || probability != 0.9 || !integer || timestamp != 1732000000000 {
			t.Fatalf("wrong values: %+v", r)
		}
	})
}

func updateRow(id int64, score float64, name string) []Value {
	return []Value{{typeID: Integer, integer: id}, {typeID: Real, real: score}, {typeID: Text, text: name}}
}

func TestUpdateErrors(t *testing.T) {
	cases := []struct{ sql, message, at string }{
		{"UPDATE missing SET id = 2 WHERE id = 1;", "Unknown table", "missing"},
		{"UPDATE sample SET name = 'new', wrong = 2 WHERE name = 'Alice';", "Unknown column", "wrong"},
		{"UPDATE sample SET name = 'Привет', Wrong = 2 WHERE name = 'Alice';", "Unknown column", "Wrong"},
		{"UPDATE sample SET name = 'new' WHERE wrong = 1;", "Unknown column", "wrong"},
		{"UPDATE sample SET name = 'new', name = 'again' WHERE id = 1;", "Duplicate UPDATE", "name = 'again'"},
		{"UPDATE sample SET name = 'new', score = 30 WHERE name = 'Alice';", "Type mismatch", "30"},
		{"UPDATE sample SET name = 'new', id = 30.0 WHERE id = 1;", "Type mismatch", "30.0"},
		{"UPDATE sample SET name = 30 WHERE id = 1;", "Type mismatch", "30"},
		{"UPDATE sample SET name = 'new' WHERE id = '1';", "Type mismatch", "'1'"},
		{"UPDATE sample SET name = 'new' WHERE score = 1;", "Type mismatch", "1;"},
		{"UPDATE sample SET name = 'new' WHERE name = 1;", "Type mismatch", "1;"},
		{"UPDATE sample SET id = 9223372036854775808 WHERE id = 1;", "64-bit", "9223372036854775808"},
		{"UPDATE sample SET id = -9223372036854775809 WHERE id = 1;", "64-bit", "-9223372036854775809"},
		{"UPDATE sample SET score = " + strings.Repeat("9", 400) + ".0 WHERE id = 1;", "finite double", strings.Repeat("9", 400)},
		{"UPDATE sample SET name = 'new' WHERE id = 9223372036854775808;", "64-bit", "9223372036854775808"},
		{"UPDATE sample SET name = 'new';", "keyword WHERE", ";"},
		{"UPDATE sample SET WHERE id = 1;", "Expected '='", "id"},
		{"UPDATE sample SET id 2 WHERE id = 1;", "Expected '='", "2"},
		{"UPDATE sample SET id = WHERE id = 1;", "literal", "WHERE"},
		{"UPDATE sample SET id = NULL WHERE id = 1;", "literal", "NULL"},
		{"UPDATE sample SET score = 1. WHERE id = 1;", "decimal point", ""},
		{"UPDATE sample SET id = 2, WHERE id = 1;", "Expected '='", "id = 1"},
		{"UPDATE sample SET id = 2 WHERE id > 1;", "Unsupported character", ">"},
		{"UPDATE sample SET id = 2 WHERE id = 1 AND name = 'Alice';", "Expected ';'", "AND"},
		{"UPDATE sample SET id = 2 WHERE id = 1 OR id = 3;", "Expected ';'", "OR"},
		{"UPDATE sample SET id = 2 WHERE id = 1", "Expected ';'", ""},
		{"UPDATE sample SET id = 2 WHERE id = 1; SELECT * FROM sample;", "end of input", "SELECT"},
		{"UPDATE sample SET name = 'broken WHERE id = 1;", "Unterminated TEXT", "'broken"},
		{"UPDATE", "identifier", ""},
	}
	for _, tc := range cases {
		t.Run(tc.sql, func(t *testing.T) {
			db := updateSample(t)
			before := execute(t, db, "SELECT * FROM sample;")
			position := -1
			if tc.at != "" {
				position = strings.Index(tc.sql, tc.at)
			}
			rejects(t, db, tc.sql, tc.message, position)
			after := execute(t, db, "SELECT * FROM sample;")
			if !reflect.DeepEqual(before, after) {
				t.Fatal("failed UPDATE changed stored rows")
			}
		})
	}
	// Validation cannot be skipped when no rows match or the table is empty.
	for _, empty := range []bool{false, true} {
		db := sample(t)
		if !empty {
			execute(t, db, "INSERT INTO sample (id, score, name) VALUES (1, 0.8, 'Alice');")
		}
		for _, sql := range []string{
			"UPDATE sample SET name = 'new', score = 30 WHERE id = 999;",
			"UPDATE sample SET name = 'new' WHERE id = '999';",
			"UPDATE sample SET name = 'new', wrong = 1 WHERE id = 999;",
		} {
			before := execute(t, db, "SELECT * FROM sample;")
			rejects(t, db, sql, "", -1)
			if !reflect.DeepEqual(before, execute(t, db, "SELECT * FROM sample;")) {
				t.Fatal("invalid zero-match UPDATE changed data")
			}
		}
	}
}
