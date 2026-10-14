package dbms

import (
	"math"
	"strconv"
	"strings"
)

type identifier struct {
	name     string
	position int
}
type literal struct {
	value    Value
	position int
}
type columnDefinition struct {
	identifier
	typeID Type
}
type predicate struct {
	column  identifier
	literal literal
}
type operation uint8

const (
	create operation = iota
	insert
	selectRows
)

type statement struct {
	operation   operation
	table       identifier
	definitions []columnDefinition
	columns     []identifier
	values      []literal
	all         bool
	predicate   *predicate
}
type parser struct {
	tokens []token
	index  int
}

func (p *parser) current() token            { return p.tokens[p.index] }
func (p *parser) fail(message string) error { return queryError(p.current().position, "%s", message) }
func (p *parser) take(k kind) bool {
	if p.current().kind != k {
		return false
	}
	p.index++
	return true
}
func (p *parser) takeKeyword(text string) bool {
	t := p.current()
	if t.kind != word || !strings.EqualFold(t.text, text) {
		return false
	}
	p.index++
	return true
}
func (p *parser) require(k kind, message string) error {
	if !p.take(k) {
		return p.fail(message)
	}
	return nil
}
func (p *parser) requireKeyword(text string) error {
	if !p.takeKeyword(text) {
		return p.fail("Expected keyword " + text)
	}
	return nil
}
func (p *parser) identifier() (identifier, error) {
	t := p.current()
	if !p.take(word) {
		return identifier{}, p.fail("Expected an identifier")
	}
	return identifier{t.text, t.position}, nil
}
func (p *parser) identifiers() ([]identifier, error) {
	var result []identifier
	for {
		id, err := p.identifier()
		if err != nil {
			return nil, err
		}
		result = append(result, id)
		if !p.take(comma) {
			return result, nil
		}
	}
}
func (p *parser) valueType() (Type, error) {
	if p.takeKeyword("INTEGER") {
		return Integer, nil
	}
	if p.takeKeyword("REAL") {
		return Real, nil
	}
	if p.takeKeyword("TEXT") {
		return Text, nil
	}
	return Integer, p.fail("Expected type INTEGER, REAL or TEXT")
}
func (p *parser) literal() (literal, error) {
	t := p.current()
	var v Value
	switch {
	case p.take(textLiteral):
		v = Value{typeID: Text, text: t.text}
	case p.take(integerLiteral):
		n, err := strconv.ParseInt(t.text, 10, 64)
		if err != nil {
			return literal{}, queryError(t.position, "INTEGER literal is outside the signed 64-bit range")
		}
		v = Value{typeID: Integer, integer: n}
	case p.take(realLiteral):
		n, err := strconv.ParseFloat(t.text, 64)
		if err != nil || math.IsInf(n, 0) || math.IsNaN(n) {
			return literal{}, queryError(t.position, "REAL literal is outside the finite double range")
		}
		v = Value{typeID: Real, real: n}
	default:
		return literal{}, p.fail("Expected an INTEGER, REAL or TEXT literal")
	}
	return literal{v, t.position}, nil
}
func parse(source string) (statement, error) {
	tokens, err := lex(source)
	if err != nil {
		return statement{}, err
	}
	p := parser{tokens: tokens}
	var s statement
	switch {
	case p.takeKeyword("CREATE"):
		s, err = p.create()
	case p.takeKeyword("INSERT"):
		s, err = p.insert()
	case p.takeKeyword("SELECT"):
		s, err = p.selectStatement()
	default:
		err = p.fail("Expected CREATE, INSERT or SELECT; statement is unsupported")
	}
	if err != nil {
		return statement{}, err
	}
	if err = p.require(semicolon, "Expected ';' at the end of the statement"); err != nil {
		return statement{}, err
	}
	if err = p.require(end, "Expected end of input after one statement"); err != nil {
		return statement{}, err
	}
	return s, nil
}
func (p *parser) create() (statement, error) {
	s := statement{operation: create}
	if err := p.requireKeyword("TABLE"); err != nil {
		return s, err
	}
	var err error
	s.table, err = p.identifier()
	if err != nil {
		return s, err
	}
	if err = p.require(leftParen, "Expected '(' before column definitions"); err != nil {
		return s, err
	}
	for {
		id, err := p.identifier()
		if err != nil {
			return s, err
		}
		typeID, err := p.valueType()
		if err != nil {
			return s, err
		}
		s.definitions = append(s.definitions, columnDefinition{id, typeID})
		if !p.take(comma) {
			break
		}
	}
	return s, p.require(rightParen, "Expected ')' after column definitions")
}
func (p *parser) insert() (statement, error) {
	s := statement{operation: insert}
	if err := p.requireKeyword("INTO"); err != nil {
		return s, err
	}
	var err error
	s.table, err = p.identifier()
	if err != nil {
		return s, err
	}
	if err = p.require(leftParen, "Expected '(' before INSERT columns"); err != nil {
		return s, err
	}
	s.columns, err = p.identifiers()
	if err != nil {
		return s, err
	}
	if err = p.require(rightParen, "Expected ')' after INSERT columns"); err != nil {
		return s, err
	}
	if err = p.requireKeyword("VALUES"); err != nil {
		return s, err
	}
	if err = p.require(leftParen, "Expected '(' before INSERT values"); err != nil {
		return s, err
	}
	for {
		v, err := p.literal()
		if err != nil {
			return s, err
		}
		s.values = append(s.values, v)
		if !p.take(comma) {
			break
		}
	}
	return s, p.require(rightParen, "Expected ')' after INSERT values")
}
func (p *parser) selectStatement() (statement, error) {
	s := statement{operation: selectRows, all: p.take(star)}
	var err error
	if !s.all {
		s.columns, err = p.identifiers()
		if err != nil {
			return s, err
		}
	}
	if err = p.requireKeyword("FROM"); err != nil {
		return s, err
	}
	s.table, err = p.identifier()
	if err != nil {
		return s, err
	}
	if p.takeKeyword("WHERE") {
		id, err := p.identifier()
		if err != nil {
			return s, err
		}
		if err = p.require(equal, "Only a single WHERE column = value condition is supported"); err != nil {
			return s, err
		}
		v, err := p.literal()
		if err != nil {
			return s, err
		}
		s.predicate = &predicate{id, v}
	}
	return s, nil
}
