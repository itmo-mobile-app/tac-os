#include "sql.hpp"

#include <charconv>
#include <cmath>
#include <locale>
#include <sstream>
#include <system_error>

namespace tacos::dbms::sql {
namespace {

bool keyword(const Token& token, std::string_view expected) {
    if (token.kind != Kind::Word || token.text.size() != expected.size()) {
        return false;
    }
    for (std::size_t i = 0; i < expected.size(); ++i) {
        char c = token.text[i];
        if (c >= 'a' && c <= 'z') {
            c = static_cast<char>(c - 'a' + 'A');
        }
        if (c != expected[i]) {
            return false;
        }
    }
    return true;
}

class Parser {
public:
    explicit Parser(std::vector<Token> tokens) : tokens_(std::move(tokens)) {}

    Statement statement() {
        Statement result;
        if (take_keyword("CREATE")) {
            result = create();
        } else if (take_keyword("INSERT")) {
            result = insert();
        } else if (take_keyword("SELECT")) {
            result = select();
        } else {
            fail("Expected CREATE, INSERT or SELECT; statement is unsupported");
        }
        require(Kind::Semicolon, "Expected ';' at the end of the statement");
        require(Kind::End, "Expected end of input after one statement");
        return result;
    }

private:
    const Token& current() const { return tokens_[index_]; }
    [[noreturn]] void fail(const std::string& message) const {
        throw QueryError(message, current().position);
    }
    bool take(Kind kind) {
        if (current().kind != kind) {
            return false;
        }
        ++index_;
        return true;
    }
    bool take_keyword(std::string_view text) {
        if (!keyword(current(), text)) {
            return false;
        }
        ++index_;
        return true;
    }
    void require(Kind kind, const std::string& message) {
        if (!take(kind)) {
            fail(message);
        }
    }
    void require_keyword(std::string_view text) {
        if (!take_keyword(text)) {
            fail("Expected keyword " + std::string(text));
        }
    }
    Identifier identifier() {
        if (current().kind != Kind::Word) {
            fail("Expected an identifier");
        }
        const Token token = tokens_[index_++];
        return {token.text, token.position};
    }
    std::size_t type() {
        if (take_keyword("INTEGER")) { return 0; }
        if (take_keyword("REAL")) { return 1; }
        if (take_keyword("TEXT")) { return 2; }
        fail("Expected type INTEGER, REAL or TEXT");
    }
    Literal literal() {
        const Token token = current();
        if (take(Kind::Text)) {
            return {token.text, token.position};
        }
        if (take(Kind::Integer)) {
            std::int64_t value = 0;
            const auto conversion = std::from_chars(token.text.data(), token.text.data() + token.text.size(), value);
            if (conversion.ec != std::errc{} || conversion.ptr != token.text.data() + token.text.size()) {
                throw QueryError("INTEGER literal is outside the signed 64-bit range", token.position);
            }
            return {value, token.position};
        }
        if (take(Kind::Real)) {
            double value = 0;
            std::istringstream input(token.text);
            input.imbue(std::locale::classic());
            input >> value;
            if (input.fail() || !std::isfinite(value)) {
                throw QueryError("REAL literal is outside the finite double range", token.position);
            }
            return {value, token.position};
        }
        fail("Expected an INTEGER, REAL or TEXT literal");
    }
    std::vector<Identifier> identifiers() {
        std::vector<Identifier> result;
        do {
            result.push_back(identifier());
        } while (take(Kind::Comma));
        return result;
    }
    Create create() {
        require_keyword("TABLE");
        Create result{identifier(), {}};
        require(Kind::LeftParen, "Expected '(' before column definitions");
        do {
            const Identifier name = identifier();
            result.columns.push_back({name, type()});
        } while (take(Kind::Comma));
        require(Kind::RightParen, "Expected ')' after column definitions");
        return result;
    }
    Insert insert() {
        require_keyword("INTO");
        Insert result{identifier(), {}, {}};
        require(Kind::LeftParen, "Expected '(' before INSERT columns");
        result.columns = identifiers();
        require(Kind::RightParen, "Expected ')' after INSERT columns");
        require_keyword("VALUES");
        require(Kind::LeftParen, "Expected '(' before INSERT values");
        do {
            result.values.push_back(literal());
        } while (take(Kind::Comma));
        require(Kind::RightParen, "Expected ')' after INSERT values");
        return result;
    }
    Select select() {
        const bool all = take(Kind::Star);
        auto columns = all ? std::vector<Identifier>{} : identifiers();
        require_keyword("FROM");
        Select result{identifier(), all, std::move(columns), std::nullopt};
        if (take_keyword("WHERE")) {
            const Identifier column = identifier();
            require(Kind::Equal, "Only a single WHERE column = value condition is supported");
            result.predicate = Predicate{column, literal()};
        }
        return result;
    }

    std::vector<Token> tokens_;
    std::size_t index_ = 0;
};

} // namespace

Statement parse(std::string_view source) { return Parser(lex(source)).statement(); }

} // namespace tacos::dbms::sql
