CREATE TABLE demo (id INTEGER, text TEXT, score REAL);
INSERT INTO demo (text, score, id)
VALUES ('O''Reilly; yes', -30.5, 1732000000000);
insert into demo (id, text, score) values (2, 'line
two', 30.0); SELECT id, text FROM demo;
SELECT * FROM demo WHERE id = 999;
UPDATE demo SET text = 'new; O''Reilly', score = 0.9 WHERE id = 2;
SELECT * FROM demo WHERE id = 2;
