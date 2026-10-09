CREATE TABLE demo (id INTEGER, text TEXT, score REAL);
INSERT INTO demo (text, score, id)
VALUES ('O''Reilly; yes', -30.5, 1732000000000);
insert into demo (id, text, score) values (2, 'line
two', 30.0); SELECT id, text FROM demo;
SELECT * FROM demo WHERE id = 999;
