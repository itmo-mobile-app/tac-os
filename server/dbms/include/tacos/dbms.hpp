#pragma once

#include <cstddef>
#include <cstdint>
#include <map>
#include <stdexcept>
#include <string>
#include <string_view>
#include <variant>
#include <vector>

namespace tacos::dbms {

using Value = std::variant<std::int64_t, double, std::string>;

// Positions are zero-based byte offsets in the supplied SQL statement.
class QueryError : public std::runtime_error {
public:
    QueryError(std::string message, std::size_t position);
    std::size_t position() const noexcept;

private:
    std::size_t position_;
};

struct Result {
    std::vector<std::string> columns;
    std::vector<std::vector<Value>> rows;
};

// This is a local executor for Sprint 0, not a Target Service client protocol.
class Database {
public:
    Result execute(std::string_view sql);

private:
    struct Column {
        std::string name;
        std::size_t value_type;
    };
    struct Table {
        std::vector<Column> columns;
        std::vector<std::vector<Value>> rows;
    };
    std::map<std::string, Table> tables_;
};

} // namespace tacos::dbms
