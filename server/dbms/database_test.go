package dbms

import (
	"errors"
	"math"
	"reflect"
	"strings"
	"testing"
)

func execute(t *testing.T, db *Database, sql string) Result {
	t.Helper()
	r, err := db.Execute(sql)
	if err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
	return r
}
func rejects(t *testing.T, db *Database, sql, message string, position int) {
	t.Helper()
	_, err := db.Execute(sql)
	var query *QueryError
	if !errors.As(err, &query) {
		t.Fatalf("expected QueryError for %q, got %v", sql, err)
	}
	if !strings.Contains(query.Message, message) {
		t.Fatalf("wrong error for %q: %v", sql, err)
	}
	if query.Position < 0 || query.Position > len(sql) {
		t.Fatalf("position outside query: %+v", query)
	}
	if position >= 0 && query.Position != position {
		t.Fatalf("position = %d, want %d for %q", query.Position, position, sql)
	}
}
func sample(t *testing.T) *Database {
	t.Helper()
	db := &Database{}
	execute(t, db, "CREATE TABLE sample (id INTEGER, score REAL, name TEXT);")
	return db
}
func TestCreate(t *testing.T) {
	db := &Database{}
	execute(t, db, "cReAtE tAbLe sample (id integer, score real, name text);")
	r := execute(t, db, "SELECT * FROM sample;")
	if !reflect.DeepEqual(r.Columns, []string{"id", "score", "name"}) || len(r.Rows) != 0 {
		t.Fatalf("wrong schema: %+v", r)
	}
	execute(t, db, "CREATE TABLE Sample (other TEXT);")
	if execute(t, db, "SELECT * FROM Sample;").Columns[0] != "other" {
		t.Fatal("identifier case lost")
	}
	rejects(t, db, "CREATE TABLE sample (x TEXT);", "already exists", 13)
	rejects(t, db, "CREATE TABLE bad (x INTEGER, x TEXT);", "Duplicate column", 29)
	rejects(t, db, "SELECT * FROM bad;", "Unknown table", -1)
	rejects(t, db, "CREATE TABLE empty ();", "identifier", -1)
	rejects(t, db, "CREATE TABLE bad (x BOOLEAN);", "Expected type", -1)
}
func TestInsert(t *testing.T) {
	db := sample(t)
	execute(t, db, "INSERT INTO sample (name, score, id) VALUES ('first', -30.5, 1732000000000);")
	execute(t, db, "INSERT INTO sample (id, score, name) VALUES (-30, 30.0, 'second');")
	r := execute(t, db, "SELECT * FROM sample;")
	if len(r.Rows) != 2 {
		t.Fatal("wrong row count")
	}
	n, ok := r.Rows[0][0].Integer()
	if !ok || n != 1732000000000 {
		t.Fatal("integer or schema order lost")
	}
	f, ok := r.Rows[0][1].Real()
	if !ok || f != -30.5 {
		t.Fatal("real or schema order lost")
	}
	s, ok := r.Rows[0][2].Text()
	if !ok || s != "first" {
		t.Fatal("text or schema order lost")
	}
	n, _ = r.Rows[1][0].Integer()
	f, ok = r.Rows[1][1].Real()
	if n != -30 || !ok || f != 30 {
		t.Fatal("negative integer or real type lost")
	}
}
func TestSelect(t *testing.T) {
	db := sample(t)
	for _, sql := range []string{
		"INSERT INTO sample (id, score, name) VALUES (1, 0.8, 'Alice');",
		"INSERT INTO sample (id, score, name) VALUES (2, 0.9, 'Bob');",
		"INSERT INTO sample (id, score, name) VALUES (3, 0.8, 'Alice');",
	} {
		execute(t, db, sql)
	}
	r := execute(t, db, "select name, id from sample where score = 0.8;")
	if !reflect.DeepEqual(r.Columns, []string{"name", "id"}) || len(r.Rows) != 2 {
		t.Fatalf("wrong projection/filter: %+v", r)
	}
	for i, want := range []int64{1, 3} {
		n, _ := r.Rows[i][1].Integer()
		s, _ := r.Rows[i][0].Text()
		if n != want || s != "Alice" {
			t.Fatal("wrong projected values")
		}
	}
	for _, tc := range []struct {
		condition string
		count     int
	}{{"id = 2", 1}, {"name = 'Alice'", 2}, {"name = 'alice'", 0}, {"id = 999", 0}} {
		if len(execute(t, db, "SELECT * FROM sample WHERE "+tc.condition+";").Rows) != tc.count {
			t.Fatalf("wrong filter: %s", tc.condition)
		}
	}
	if len(execute(t, db, "SELECT id, id FROM sample WHERE id = 1;").Rows[0]) != 2 {
		t.Fatal("duplicate projection rejected")
	}
}
func TestLiterals(t *testing.T) {
	db := &Database{}
	execute(t, db, "CREATE TABLE bounds (n INTEGER, r REAL, text TEXT);")
	execute(t, db, "INSERT INTO bounds (n, r, text) VALUES (-9223372036854775808, -0.5, 'O''Reilly; SELECT');")
	execute(t, db, "INSERT INTO bounds (n, r, text) VALUES (9223372036854775807, 30.0, 'a\\b');")
	execute(t, db, "INSERT INTO bounds (n, r, text) VALUES (0, -0.0, '');")
	execute(t, db, "INSERT INTO bounds (n, r, text) VALUES (1, 1.0, 'line\none Привет');")
	r := execute(t, db, "SELECT * FROM bounds;")
	a, _ := r.Rows[0][0].Integer()
	b, _ := r.Rows[1][0].Integer()
	if a != math.MinInt64 || b != math.MaxInt64 {
		t.Fatal("integer bounds lost")
	}
	for i, want := range []string{"O'Reilly; SELECT", "a\\b", "", "line\none Привет"} {
		got, _ := r.Rows[i][2].Text()
		if got != want {
			t.Fatalf("TEXT = %q, want %q", got, want)
		}
	}
	rejects(t, db, "INSERT INTO bounds (n, r, text) VALUES (9223372036854775808, 0.0, 'x');", "64-bit", -1)
	rejects(t, db, "INSERT INTO bounds (n, r, text) VALUES (-9223372036854775809, 0.0, 'x');", "64-bit", -1)
	rejects(t, db, "INSERT INTO bounds (n, r, text) VALUES (0, "+strings.Repeat("9", 400)+".0, 'x');", "finite double", -1)
	// Very small finite literals round to zero, consistent with IEEE-754 storage.
	execute(t, db, "INSERT INTO bounds (n, r, text) VALUES (2, 0."+strings.Repeat("0", 400)+"1, 'tiny');")
}
func TestSyntax(t *testing.T) {
	db := sample(t)
	for _, tc := range []struct {
		sql, message string
		position     int
	}{
		{"SELECT * FROM sample", "Expected ';'", 20},
		{"SELECT * FROM sample; SELECT * FROM sample;", "end of input", 22},
		{"SELECT FROM sample;", "keyword FROM", -1},
		{"SELECT id, FROM sample;", "keyword FROM", -1},
		{"SELECT * FROM sample WHERE id > 1;", "Unsupported character", 30},
		{"SELECT * FROM sample WHERE id = 1 AND name = 'x';", "Expected ';'", 34},
		{"SELECT * FROM sample JOIN other;", "Expected ';'", 21},
		{"DELETE FROM sample;", "unsupported", 0},
		{"INSERT INTO sample (id, score, name) VALUES (+1, 1.0, 'x');", "Unsupported character", -1},
		{"INSERT INTO sample (id, score, name) VALUES (1, .5, 'x');", "Unsupported character", -1},
		{"INSERT INTO sample (id, score, name) VALUES (1, 1., 'x');", "decimal point", -1},
		{"INSERT INTO sample (id, score, name) VALUES (1, 1e2, 'x');", "Expected ')'", -1},
		{"INSERT INTO sample (id, score, name) VALUES (1, 1.0, NULL);", "literal", -1},
		{"", "unsupported", 0},
		{"CREATE TABLE x (id INTEGER); garbage", "end of input", -1},
		{"CREATE TABLE x (id INTEGER) garbage;", "Expected ';'", -1},
		{"CREATE TABLE", "identifier", -1},
		{"INSERT", "keyword INTO", -1},
		{"SELECT", "identifier", -1},
	} {
		t.Run(tc.sql, func(t *testing.T) { rejects(t, db, tc.sql, tc.message, tc.position) })
	}
	sql := "INSERT INTO sample (id, score, name) VALUES (1, 1.0, 'broken);"
	rejects(t, db, sql, "Unterminated TEXT", strings.IndexByte(sql, '\''))
}
func TestSemantic(t *testing.T) {
	db := sample(t)
	for _, tc := range []struct {
		sql, message string
		position     int
	}{
		{"INSERT INTO sample (id, score, name) VALUES (1.0, 1.0, 'x');", "Type mismatch", -1},
		{"INSERT INTO sample (id, score, name) VALUES (1, 1, 'x');", "Type mismatch", -1},
		{"INSERT INTO sample (id, score, name) VALUES (1, 1.0, 2);", "Type mismatch", -1},
		{"INSERT INTO sample (id, score, name) VALUES (1, 1.0);", "count", -1},
		{"INSERT INTO sample (id, name) VALUES (1, 'x');", "every column", -1},
		{"INSERT INTO sample (id, score, id) VALUES (1, 1.0, 2);", "Duplicate INSERT", -1},
		{"INSERT INTO sample (id, score, wrong) VALUES (1, 1.0, 'x');", "Unknown column", -1},
		{"INSERT INTO missing (id) VALUES (1);", "Unknown table", -1},
		{"SELECT wrong FROM sample;", "Unknown column", 7},
		{"SELECT * FROM missing;", "Unknown table", 14},
		{"SELECT * FROM sample WHERE wrong = 1;", "Unknown column", 27},
		{"SELECT * FROM sample WHERE id = '1';", "Type mismatch", 32},
		{"SELECT * FROM sample WHERE score = 1;", "Type mismatch", -1},
	} {
		t.Run(tc.sql, func(t *testing.T) { rejects(t, db, tc.sql, tc.message, tc.position) })
	}
}
func TestAtomicity(t *testing.T) {
	db := sample(t)
	execute(t, db, "INSERT INTO sample (id, score, name) VALUES (1, 1.0, 'safe');")
	rejects(t, db, "INSERT INTO sample (id, score, name) VALUES (2, 2.0, 3);", "Type mismatch", -1)
	rejects(t, db, "INSERT INTO sample (id, score, name) VALUES (2, 2.0, 'bad'); SELECT * FROM sample;", "end of input", -1)
	if len(execute(t, db, "SELECT * FROM sample;").Rows) != 1 {
		t.Fatal("failed INSERT changed data")
	}
	rejects(t, db, "CREATE TABLE broken (x INTEGER, x TEXT);", "Duplicate column", -1)
	execute(t, db, "CREATE TABLE broken (x INTEGER);")
	if len(execute(t, db, "SELECT * FROM broken;").Rows) != 0 {
		t.Fatal("failed CREATE left data")
	}
}
func TestIndependent(t *testing.T) {
	first := sample(t)
	second := &Database{}
	rejects(t, second, "SELECT * FROM sample;", "Unknown table", -1)
	execute(t, first, "INSERT INTO sample (id, score, name) VALUES (1, 1.0, 'x');")
	r := execute(t, first, "SELECT * FROM sample;")
	r.Rows[0][2] = Value{typeID: Text, text: "changed"}
	r.Columns[0] = "wrong"
	s, _ := execute(t, first, "SELECT name FROM sample;").Rows[0][0].Text()
	if s != "x" {
		t.Fatal("result mutation changed stored data")
	}
}
