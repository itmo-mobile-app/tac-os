CREATE TABLE demo (x INTEGER);
DELETE FROM demo;
INSERT INTO demo (x) VALUES (7);
UPDATE demo SET x = 'wrong' WHERE x = 7;
SELECT * FROM demo;
UPDATE demo SET x = 8 WHERE x = 7;
SELECT * FROM demo;
