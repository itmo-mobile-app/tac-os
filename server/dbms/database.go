package dbms

type column struct {
	name   string
	typeID Type
}
type table struct {
	columns []column
	rows    [][]Value
}

// Database is an independent in-memory executor. It is not safe for concurrent use.
// Its zero value is ready to use. Copying a used Database is unsupported.
type Database struct{ tables map[string]*table }

// Execute parses exactly one complete statement before changing any stored data.
func (db *Database) Execute(source string) (Result, error) {
	s, err := parse(source)
	if err != nil {
		return Result{}, err
	}
	if s.operation == create {
		if _, ok := db.tables[s.table.name]; ok {
			return Result{}, queryError(s.table.position, "Table already exists: %s", s.table.name)
		}
		t := &table{}
		names := make(map[string]bool)
		for _, c := range s.definitions {
			if names[c.name] {
				return Result{}, queryError(c.position, "Duplicate column: %s", c.name)
			}
			names[c.name] = true
			t.columns = append(t.columns, column{c.name, c.typeID})
		}
		if db.tables == nil {
			db.tables = make(map[string]*table)
		}
		db.tables[s.table.name] = t
		return Result{}, nil
	}
	t, ok := db.tables[s.table.name]
	if !ok {
		return Result{}, queryError(s.table.position, "Unknown table: %s", s.table.name)
	}
	if s.operation == insert {
		return t.insert(s)
	}
	return t.selectRows(s)
}
func (t *table) columnIndex(id identifier) (int, error) {
	for i, c := range t.columns {
		if c.name == id.name {
			return i, nil
		}
	}
	return 0, queryError(id.position, "Unknown column: %s", id.name)
}
func (t *table) checkType(index int, value literal) error {
	if t.columns[index].typeID != value.value.typeID {
		return queryError(value.position, "Type mismatch for column: %s", t.columns[index].name)
	}
	return nil
}
func (t *table) insert(s statement) (Result, error) {
	if len(s.columns) != len(s.values) {
		return Result{}, queryError(s.table.position, "INSERT column count does not match value count")
	}
	seen := make(map[int]bool)
	row := make([]Value, len(t.columns))
	for i, c := range s.columns {
		index, err := t.columnIndex(c)
		if err != nil {
			return Result{}, err
		}
		if seen[index] {
			return Result{}, queryError(c.position, "Duplicate INSERT column: %s", c.name)
		}
		seen[index] = true
		if err = t.checkType(index, s.values[i]); err != nil {
			return Result{}, err
		}
		row[index] = s.values[i].value
	}
	if len(seen) != len(t.columns) {
		return Result{}, queryError(s.table.position, "INSERT must provide every column exactly once; NULL/defaults are unsupported")
	}
	t.rows = append(t.rows, row)
	return Result{}, nil
}
func (t *table) selectRows(s statement) (Result, error) {
	var selected []int
	result := Result{}
	if s.all {
		for i, c := range t.columns {
			selected = append(selected, i)
			result.Columns = append(result.Columns, c.name)
		}
	} else {
		for _, c := range s.columns {
			i, err := t.columnIndex(c)
			if err != nil {
				return Result{}, err
			}
			selected = append(selected, i)
			result.Columns = append(result.Columns, c.name)
		}
	}
	filter := -1
	if s.predicate != nil {
		var err error
		filter, err = t.columnIndex(s.predicate.column)
		if err != nil {
			return Result{}, err
		}
		if err = t.checkType(filter, s.predicate.literal); err != nil {
			return Result{}, err
		}
	}
	for _, row := range t.rows {
		if filter >= 0 && row[filter] != s.predicate.literal.value {
			continue
		}
		projected := make([]Value, len(selected))
		for i, index := range selected {
			projected[i] = row[index]
		}
		result.Rows = append(result.Rows, projected)
	}
	return result, nil
}
