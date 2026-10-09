#include "tacos/dbms.hpp"

#include <iomanip>
#include <iostream>
#include <limits>
#include <locale>
#include <optional>
#include <sstream>
#include <type_traits>
#include <utility>

namespace {

bool whitespace(char c) {
    return c == ' ' || c == '\t' || c == '\r' || c == '\n' || c == '\f' || c == '\v';
}

std::optional<std::string> read_statement(std::istream& input) {
    std::string source;
    bool quoted = false;
    bool has_content = false;
    char c;
    while (input.get(c)) {
        source += c;
        has_content = has_content || !whitespace(c);
        if (c == '\'') {
            if (quoted && input.peek() == '\'') {
                source += static_cast<char>(input.get());
            } else {
                quoted = !quoted;
            }
        } else if (c == ';' && !quoted) {
            return source;
        }
    }
    return has_content ? std::optional<std::string>(std::move(source)) : std::nullopt;
}

std::string display(const tacos::dbms::Value& value) {
    return std::visit([](const auto& item) -> std::string {
        using T = std::decay_t<decltype(item)>;
        if constexpr (std::is_same_v<T, std::string>) {
            std::string text = "'";
            for (const char c : item) {
                switch (c) {
                case '\'': text += "''"; break;
                case '\\': text += "\\\\"; break;
                case '\n': text += "\\n"; break;
                case '\r': text += "\\r"; break;
                case '\t': text += "\\t"; break;
                default: text += c; break;
                }
            }
            return text + "'";
        } else {
            std::ostringstream text;
            text.imbue(std::locale::classic());
            if constexpr (std::is_same_v<T, double>) {
                text << std::setprecision(std::numeric_limits<double>::max_digits10);
            }
            text << item;
            return text.str();
        }
    }, value);
}

void print(const tacos::dbms::Result& result) {
    if (result.columns.empty()) {
        std::cout << "OK\n";
        return;
    }
    for (std::size_t i = 0; i < result.columns.size(); ++i) {
        if (i != 0) { std::cout << '\t'; }
        std::cout << result.columns[i];
    }
    std::cout << '\n';
    for (const auto& row : result.rows) {
        for (std::size_t i = 0; i < row.size(); ++i) {
            if (i != 0) { std::cout << '\t'; }
            std::cout << display(row[i]);
        }
        std::cout << '\n';
    }
    std::cout << result.rows.size() << " row(s)\n";
}

} // namespace

int main() {
    tacos::dbms::Database database;
    bool failed = false;
    while (const auto statement = read_statement(std::cin)) {
        try {
            print(database.execute(*statement));
        } catch (const tacos::dbms::QueryError& error) {
            // Human-facing positions are one-based, unlike the C++ API.
            std::cerr << "ERROR at position " << error.position() + 1 << ": " << error.what() << '\n';
            failed = true;
        }
    }
    if (std::cin.bad()) {
        std::cerr << "ERROR: could not read standard input\n";
        return 1;
    }
    return failed ? 1 : 0;
}
