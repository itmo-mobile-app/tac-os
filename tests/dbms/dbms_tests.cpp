#include "tacos/dbms.hpp"

#include <functional>
#include <iostream>
#include <limits>
#include <map>
#include <string>

namespace {

using tacos::dbms::Database;
using tacos::dbms::QueryError;
using tacos::dbms::Result;
using tacos::dbms::Value;

void check(bool condition, const std::string& message) {
    if (!condition) {
        throw std::runtime_error(message);
    }
}

void rejects(Database& db, const std::string& query, const std::string& message,
             std::size_t position = std::string::npos) {
    try {
        db.execute(query);
    } catch (const QueryError& error) {
        check(std::string(error.what()).find(message) != std::string::npos,
              "Wrong error for: " + query + " -> " + error.what());
        check(error.position() <= query.size(), "Error position is outside input");
        if (position != std::string::npos) {
            check(error.position() == position, "Wrong error position for: " + query);
        }
        return;
    }
    throw std::runtime_error("Query unexpectedly succeeded: " + query);
}

Database sample() {
    Database db;
    db.execute("CREATE TABLE sample (id INTEGER, score REAL, name TEXT);");
    return db;
}

void test_create() {
    Database db;
    db.execute("cReAtE tAbLe sample (id integer, score real, name text);");
    const Result result = db.execute("SELECT * FROM sample;");
    check(result.columns == std::vector<std::string>({"id", "score", "name"}), "Wrong CREATE schema");
    check(result.rows.empty(), "New table should be empty");
    db.execute("CREATE TABLE Sample (other TEXT);");
    check(db.execute("SELECT * FROM Sample;").columns[0] == "other", "Identifiers must preserve case");
    rejects(db, "CREATE TABLE sample (x TEXT);", "already exists", 13);
    rejects(db, "CREATE TABLE bad (x INTEGER, x TEXT);", "Duplicate column", 29);
    rejects(db, "SELECT * FROM bad;", "Unknown table");
    rejects(db, "CREATE TABLE empty ();", "identifier");
    rejects(db, "CREATE TABLE bad (x BOOLEAN);", "Expected type");
}

void test_insert() {
    Database db = sample();
    db.execute("INSERT INTO sample (name, score, id) VALUES ('first', -30.5, 1732000000000);");
    db.execute("INSERT INTO sample (id, score, name) VALUES (-30, 30.0, 'second');");
    const Result result = db.execute("SELECT * FROM sample;");
    check(result.rows.size() == 2, "Wrong inserted row count");
    check(result.rows[0] == std::vector<Value>({std::int64_t{1732000000000}, -30.5, std::string("first")}),
          "INSERT should use schema order, not argument order");
    check(std::get<std::int64_t>(result.rows[1][0]) == -30, "Negative INTEGER lost");
    check(std::get<double>(result.rows[1][1]) == 30.0, "Wrong REAL value");
}

void test_select() {
    Database db = sample();
    db.execute("INSERT INTO sample (id, score, name) VALUES (1, 0.8, 'Alice');");
    db.execute("INSERT INTO sample (id, score, name) VALUES (2, 0.9, 'Bob');");
    db.execute("INSERT INTO sample (id, score, name) VALUES (3, 0.8, 'Alice');");
    const Result selected = db.execute("select name, id from sample where score = 0.8;");
    check(selected.columns == std::vector<std::string>({"name", "id"}), "Wrong SELECT projection");
    check(selected.rows == std::vector<std::vector<Value>>({
        {std::string("Alice"), std::int64_t{1}}, {std::string("Alice"), std::int64_t{3}}
    }), "Wrong equality filter");
    check(db.execute("SELECT * FROM sample WHERE id = 2;").rows.size() == 1, "INTEGER WHERE failed");
    check(db.execute("SELECT * FROM sample WHERE name = 'Alice';").rows.size() == 2, "TEXT WHERE failed");
    check(db.execute("SELECT * FROM sample WHERE name = 'alice';").rows.empty(), "TEXT case lost");
    const auto missing = db.execute("SELECT id FROM sample WHERE id = 999;");
    check(missing.rows.empty() && missing.columns == std::vector<std::string>({"id"}), "Wrong empty result");
    check(db.execute("SELECT id, id FROM sample WHERE id = 1;").rows[0].size() == 2, "Repeated projection failed");
}

void test_literals() {
    Database db;
    db.execute("CREATE TABLE bounds (n INTEGER, r REAL, text TEXT);");
    db.execute("INSERT INTO bounds (n, r, text) VALUES (-9223372036854775808, -0.5, 'O''Reilly; SELECT');");
    db.execute("INSERT INTO bounds (n, r, text) VALUES (9223372036854775807, 30.0, 'a\\b');");
    db.execute("INSERT INTO bounds (n, r, text) VALUES (0, -0.0, '');");
    db.execute("INSERT INTO bounds (n, r, text) VALUES (1, 1.0, 'line\none');");
    const auto result = db.execute("SELECT * FROM bounds;");
    check(std::get<std::int64_t>(result.rows[0][0]) == std::numeric_limits<std::int64_t>::min(), "Lost INT64_MIN");
    check(std::get<std::int64_t>(result.rows[1][0]) == std::numeric_limits<std::int64_t>::max(), "Lost INT64_MAX");
    check(std::get<std::string>(result.rows[0][2]) == "O'Reilly; SELECT", "Quote or semicolon lost");
    check(std::get<std::string>(result.rows[1][2]) == "a\\b", "Backslash must be literal");
    check(std::get<std::string>(result.rows[2][2]).empty(), "Empty TEXT failed");
    check(std::get<std::string>(result.rows[3][2]) == "line\none", "Multiline TEXT failed");
    rejects(db, "INSERT INTO bounds (n, r, text) VALUES (9223372036854775808, 0.0, 'x');", "64-bit");
    rejects(db, "INSERT INTO bounds (n, r, text) VALUES (-9223372036854775809, 0.0, 'x');", "64-bit");
    rejects(db, "INSERT INTO bounds (n, r, text) VALUES (0, " + std::string(400, '9') + ".0, 'x');", "finite double");
}

void test_syntax() {
    Database db = sample();
    rejects(db, "SELECT * FROM sample", "Expected ';'", 20);
    rejects(db, "SELECT * FROM sample; SELECT * FROM sample;", "end of input", 22);
    rejects(db, "SELECT FROM sample;", "keyword FROM");
    rejects(db, "SELECT id, FROM sample;", "keyword FROM");
    rejects(db, "SELECT * FROM sample WHERE id > 1;", "Unsupported character", 30);
    rejects(db, "SELECT * FROM sample WHERE id = 1 AND name = 'x';", "Expected ';'", 34);
    rejects(db, "SELECT * FROM sample JOIN other;", "Expected ';'", 21);
    rejects(db, "UPDATE sample SET id = 2 WHERE id = 1;", "unsupported", 0);
    rejects(db, "DELETE FROM sample;", "unsupported", 0);
    rejects(db, "INSERT INTO sample (id, score, name) VALUES (+1, 1.0, 'x');", "Unsupported character");
    rejects(db, "INSERT INTO sample (id, score, name) VALUES (1, .5, 'x');", "Unsupported character");
    rejects(db, "INSERT INTO sample (id, score, name) VALUES (1, 1., 'x');", "decimal point");
    rejects(db, "INSERT INTO sample (id, score, name) VALUES (1, 1e2, 'x');", "Expected ')'");
    rejects(db, "INSERT INTO sample (id, score, name) VALUES (1, 1.0, NULL);", "literal");
    const std::string quote = "INSERT INTO sample (id, score, name) VALUES (1, 1.0, 'broken);";
    rejects(db, quote, "Unterminated TEXT", quote.find('\''));
    rejects(db, "", "unsupported", 0);
}

void test_semantic() {
    Database db = sample();
    rejects(db, "INSERT INTO sample (id, score, name) VALUES (1.0, 1.0, 'x');", "Type mismatch");
    rejects(db, "INSERT INTO sample (id, score, name) VALUES (1, 1, 'x');", "Type mismatch");
    rejects(db, "INSERT INTO sample (id, score, name) VALUES (1, 1.0, 2);", "Type mismatch");
    rejects(db, "INSERT INTO sample (id, score, name) VALUES (1, 1.0);", "count");
    rejects(db, "INSERT INTO sample (id, name) VALUES (1, 'x');", "every column");
    rejects(db, "INSERT INTO sample (id, score, id) VALUES (1, 1.0, 2);", "Duplicate INSERT");
    rejects(db, "INSERT INTO sample (id, score, wrong) VALUES (1, 1.0, 'x');", "Unknown column");
    rejects(db, "INSERT INTO missing (id) VALUES (1);", "Unknown table");
    rejects(db, "SELECT wrong FROM sample;", "Unknown column", 7);
    rejects(db, "SELECT * FROM missing;", "Unknown table", 14);
    rejects(db, "SELECT * FROM sample WHERE wrong = 1;", "Unknown column", 27);
    rejects(db, "SELECT * FROM sample WHERE id = '1';", "Type mismatch", 32);
    rejects(db, "SELECT * FROM sample WHERE score = 1;", "Type mismatch");
}

void test_atomicity() {
    Database db = sample();
    db.execute("INSERT INTO sample (id, score, name) VALUES (1, 1.0, 'safe');");
    rejects(db, "INSERT INTO sample (id, score, name) VALUES (2, 2.0, 3);", "Type mismatch");
    rejects(db, "INSERT INTO sample (id, score, name) VALUES (2, 2.0, 'bad'); SELECT * FROM sample;", "end of input");
    check(db.execute("SELECT * FROM sample;").rows.size() == 1, "Failed INSERT changed stored data");
    rejects(db, "CREATE TABLE broken (x INTEGER, x TEXT);", "Duplicate column");
    db.execute("CREATE TABLE broken (x INTEGER);");
    check(db.execute("SELECT * FROM broken;").rows.empty(), "Failed CREATE left a table");
}

void test_independent() {
    Database first = sample();
    Database second;
    rejects(second, "SELECT * FROM sample;", "Unknown table");
    first.execute("INSERT INTO sample (id, score, name) VALUES (1, 1.0, 'x');");
    auto result = first.execute("SELECT * FROM sample;");
    result.rows[0][2] = std::string("changed");
    check(std::get<std::string>(first.execute("SELECT name FROM sample;").rows[0][0]) == "x",
          "Result should not allow mutation of stored data");
}

} // namespace

int main(int argc, char** argv) {
    const std::map<std::string, std::function<void()>> cases = {
        {"create", test_create}, {"insert", test_insert}, {"select", test_select}, {"literals", test_literals},
        {"syntax", test_syntax}, {"semantic", test_semantic}, {"atomicity", test_atomicity},
        {"independent", test_independent}
    };
    if (argc != 2 || cases.count(argv[1]) == 0) {
        std::cerr << "Expected a known test case name\n";
        return 2;
    }
    try {
        cases.at(argv[1])();
        std::cout << "PASS: " << argv[1] << '\n';
        return 0;
    } catch (const std::exception& error) {
        std::cerr << "FAIL: " << error.what() << '\n';
        return 1;
    }
}
