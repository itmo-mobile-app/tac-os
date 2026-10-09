#pragma once

#include "tacos/dbms.hpp"

#include <optional>
#include <utility>

namespace tacos::dbms::sql {

enum class Kind { Word, Integer, Real, Text, LeftParen, RightParen, Comma, Equal, Star, Semicolon, End };

struct Token {
    Kind kind;
    std::string text;
    std::size_t position;
};

struct Identifier {
    std::string name;
    std::size_t position;
};

struct Literal {
    Value value;
    std::size_t position;
};

struct ColumnDefinition {
    Identifier identifier;
    std::size_t value_type;
};

struct Create {
    Identifier table;
    std::vector<ColumnDefinition> columns;
};

struct Insert {
    Identifier table;
    std::vector<Identifier> columns;
    std::vector<Literal> values;
};

struct Predicate {
    Identifier column;
    Literal literal;
};

struct Select {
    Identifier table;
    bool all_columns;
    std::vector<Identifier> columns;
    std::optional<Predicate> predicate;
};

using Statement = std::variant<Create, Insert, Select>;

std::vector<Token> lex(std::string_view source);
Statement parse(std::string_view source);

} // namespace tacos::dbms::sql
