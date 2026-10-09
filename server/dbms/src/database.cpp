#include "sql.hpp"

#include <set>
#include <type_traits>

namespace tacos::dbms {

QueryError::QueryError(std::string message, std::size_t position)
    : std::runtime_error(std::move(message)), position_(position) {}

std::size_t QueryError::position() const noexcept { return position_; }

Result Database::execute(std::string_view source) {
    // Finish parsing before performing any mutation.
    const sql::Statement statement = sql::parse(source);
    return std::visit([this](const auto& command) -> Result {
        using Command = std::decay_t<decltype(command)>;
        if constexpr (std::is_same_v<Command, sql::Create>) {
            if (tables_.count(command.table.name) != 0) {
                throw QueryError("Table already exists: " + command.table.name, command.table.position);
            }
            Table table;
            std::set<std::string> names;
            for (const auto& column : command.columns) {
                if (!names.insert(column.identifier.name).second) {
                    throw QueryError("Duplicate column: " + column.identifier.name, column.identifier.position);
                }
                table.columns.push_back({column.identifier.name, column.value_type});
            }
            tables_.emplace(command.table.name, std::move(table));
            return {};
        } else {
            const auto found = tables_.find(command.table.name);
            if (found == tables_.end()) {
                throw QueryError("Unknown table: " + command.table.name, command.table.position);
            }
            Table& table = found->second;
            const auto column_index = [&table](const sql::Identifier& identifier) {
                for (std::size_t i = 0; i < table.columns.size(); ++i) {
                    if (table.columns[i].name == identifier.name) {
                        return i;
                    }
                }
                throw QueryError("Unknown column: " + identifier.name, identifier.position);
            };
            const auto check_type = [&table](std::size_t column, const sql::Literal& literal) {
                if (table.columns[column].value_type != literal.value.index()) {
                    throw QueryError("Type mismatch for column: " + table.columns[column].name, literal.position);
                }
            };
            if constexpr (std::is_same_v<Command, sql::Insert>) {
                if (command.columns.size() != command.values.size()) {
                    throw QueryError("INSERT column count does not match value count", command.table.position);
                }
                std::set<std::size_t> seen;
                std::vector<Value> row(table.columns.size());
                for (std::size_t i = 0; i < command.columns.size(); ++i) {
                    const std::size_t column = column_index(command.columns[i]);
                    if (!seen.insert(column).second) {
                        throw QueryError("Duplicate INSERT column: " + command.columns[i].name,
                                         command.columns[i].position);
                    }
                    check_type(column, command.values[i]);
                    row[column] = command.values[i].value;
                }
                if (seen.size() != table.columns.size()) {
                    throw QueryError("INSERT must provide every column exactly once; NULL/defaults are unsupported",
                                     command.table.position);
                }
                table.rows.push_back(std::move(row));
                return {};
            } else {
                std::vector<std::size_t> selected;
                Result result;
                if (command.all_columns) {
                    for (std::size_t i = 0; i < table.columns.size(); ++i) {
                        selected.push_back(i);
                        result.columns.push_back(table.columns[i].name);
                    }
                } else {
                    for (const auto& column : command.columns) {
                        selected.push_back(column_index(column));
                        result.columns.push_back(column.name);
                    }
                }
                std::optional<std::size_t> filter_column;
                if (command.predicate) {
                    filter_column = column_index(command.predicate->column);
                    check_type(*filter_column, command.predicate->literal);
                }
                for (const auto& row : table.rows) {
                    if (filter_column && row[*filter_column] != command.predicate->literal.value) {
                        continue;
                    }
                    std::vector<Value> projected;
                    for (const auto column : selected) {
                        projected.push_back(row[column]);
                    }
                    result.rows.push_back(std::move(projected));
                }
                return result;
            }
        }
    }, statement);
}

} // namespace tacos::dbms
