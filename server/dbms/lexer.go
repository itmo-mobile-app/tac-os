package dbms

type kind uint8

const (
	word kind = iota
	integerLiteral
	realLiteral
	textLiteral
	leftParen
	rightParen
	comma
	equal
	star
	semicolon
	end
)

type token struct {
	kind     kind
	text     string
	position int
}

func digit(c byte) bool  { return c >= '0' && c <= '9' }
func letter(c byte) bool { return c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c == '_' }
func whitespace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\r' || c == '\n' || c == '\f' || c == '\v'
}

func lex(source string) ([]token, error) {
	var tokens []token
	for i := 0; i < len(source); {
		c := source[i]
		if whitespace(c) {
			i++
			continue
		}
		start := i
		if letter(c) {
			for i < len(source) && (letter(source[i]) || digit(source[i])) {
				i++
			}
			tokens = append(tokens, token{word, source[start:i], start})
			continue
		}
		if digit(c) || c == '-' {
			if c == '-' {
				i++
				if i == len(source) || !digit(source[i]) {
					return nil, queryError(start, "Expected digits after '-'")
				}
			}
			for i < len(source) && digit(source[i]) {
				i++
			}
			k := integerLiteral
			if i < len(source) && source[i] == '.' {
				k = realLiteral
				i++
				if i == len(source) || !digit(source[i]) {
					return nil, queryError(i, "Expected digits after decimal point")
				}
				for i < len(source) && digit(source[i]) {
					i++
				}
			}
			tokens = append(tokens, token{k, source[start:i], start})
			continue
		}
		if c == '\'' {
			i++
			var decoded []byte
			closed := false
			for i < len(source) {
				if source[i] != '\'' {
					decoded = append(decoded, source[i])
					i++
				} else if i+1 < len(source) && source[i+1] == '\'' {
					decoded = append(decoded, '\'')
					i += 2
				} else {
					i++
					closed = true
					break
				}
			}
			if !closed {
				return nil, queryError(start, "Unterminated TEXT literal")
			}
			tokens = append(tokens, token{textLiteral, string(decoded), start})
			continue
		}
		var k kind
		switch c {
		case '(':
			k = leftParen
		case ')':
			k = rightParen
		case ',':
			k = comma
		case '=':
			k = equal
		case '*':
			k = star
		case ';':
			k = semicolon
		default:
			return nil, queryError(start, "Unsupported character")
		}
		tokens = append(tokens, token{k, source[i : i+1], start})
		i++
	}
	return append(tokens, token{end, "", len(source)}), nil
}
