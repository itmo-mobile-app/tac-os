#include "sql.hpp"

namespace tacos::dbms::sql {
namespace {

bool digit(char c) { return c >= '0' && c <= '9'; }
bool letter(char c) { return (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || c == '_'; }
bool space(char c) { return c == ' ' || c == '\t' || c == '\r' || c == '\n' || c == '\f' || c == '\v'; }

} // namespace

std::vector<Token> lex(std::string_view source) {
    std::vector<Token> tokens;
    std::size_t i = 0;
    while (i < source.size()) {
        const char c = source[i];
        if (space(c)) {
            ++i;
            continue;
        }
        const std::size_t start = i;
        if (letter(c)) {
            while (i < source.size() && (letter(source[i]) || digit(source[i]))) {
                ++i;
            }
            tokens.push_back({Kind::Word, std::string(source.substr(start, i - start)), start});
            continue;
        }
        if (digit(c) || c == '-') {
            if (c == '-') {
                ++i;
                if (i == source.size() || !digit(source[i])) {
                    throw QueryError("Expected digits after '-'", start);
                }
            }
            while (i < source.size() && digit(source[i])) {
                ++i;
            }
            Kind kind = Kind::Integer;
            if (i < source.size() && source[i] == '.') {
                kind = Kind::Real;
                ++i;
                if (i == source.size() || !digit(source[i])) {
                    throw QueryError("Expected digits after decimal point", i);
                }
                while (i < source.size() && digit(source[i])) {
                    ++i;
                }
            }
            tokens.push_back({kind, std::string(source.substr(start, i - start)), start});
            continue;
        }
        if (c == '\'') {
            ++i;
            std::string text;
            bool closed = false;
            while (i < source.size()) {
                if (source[i] != '\'') {
                    text += source[i++];
                } else if (i + 1 < source.size() && source[i + 1] == '\'') {
                    text += '\'';
                    i += 2;
                } else {
                    ++i;
                    closed = true;
                    break;
                }
            }
            if (!closed) {
                throw QueryError("Unterminated TEXT literal", start);
            }
            tokens.push_back({Kind::Text, std::move(text), start});
            continue;
        }
        Kind kind;
        switch (c) {
        case '(': kind = Kind::LeftParen; break;
        case ')': kind = Kind::RightParen; break;
        case ',': kind = Kind::Comma; break;
        case '=': kind = Kind::Equal; break;
        case '*': kind = Kind::Star; break;
        case ';': kind = Kind::Semicolon; break;
        default: throw QueryError("Unsupported character", start);
        }
        tokens.push_back({kind, std::string(1, c), start});
        ++i;
    }
    tokens.push_back({Kind::End, "", source.size()});
    return tokens;
}

} // namespace tacos::dbms::sql
