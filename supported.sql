-- basic queries
[x] INSERT INTO `User` (Username, Age, City) VALUES ('JaneSmith', 25, 'Los Angeles'), ('AliceBrown', 28, 'Chicago'), ('BobWhite', 35, 'Houston')--[x]
[x] CREATE TABLE `User`(PRIMARY KEY(UserId), Username VARCHAR, Age INT, City VARCHAR) --[x]
[x] CREATE TABLE `User`(PRIMARY KEY(UserId), Email VARCHAR, Password VARCHAR, DbName VARCHAR)
[x] DELETE FROM `User` WHERE Username = 'JaneSmith' --[x]
[x] UPDATE `User` SET Age = 121209 WHERE Username = 'JaneSmith' --[x]

[x] SELECT * FROM `User` --[x]
[x] SELECT Username, Age FROM `User` --[x]

[x] SELECT Username, Age, City FROM `User` WHERE Age > 20 --[x]
[x] SELECT Username, Age, City FROM `User` WHERE Age = 20 --[x]
[x] SELECT Username, Age, City FROM `User` WHERE Age < 20 --[x]
[x] SELECT * FROM `User` WHERE UserId = CAST('10084632547061476038' AS DECIMAL(20,0)) --[x]
[x] SELECT Username, Age, City FROM `User` WHERE Age BETWEEN 20 AND 30 --[x]

[x] SELECT Username, Age, City FROM `User` ORDER BY Age ASC --[x]
[x] SELECT Username, Age, City FROM `User` ORDER BY Age DESC --[x]
[x] SELECT Username, Age, City FROM `User` ORDER BY Age ASC LIMIT 1 --[x]
[x] SELECT Username, Age, City FROM `User` ORDER BY Age DESC LIMIT 1 --[x]

[x] SELECT City, COUNT(*) AS UserCount FROM `User` GROUP BY City --[x]
[x] SELECT City, MAX(Age) AS max_age FROM `User` GROUP BY City --[x]
[x] SELECT City, MIN(Age) AS max_age FROM `User` GROUP BY City --[x]
[x] SELECT City, AVG(Age) AS max_age FROM `User` GROUP BY City --[x]
[x] SELECT City, SUM(Age) AS max_age FROM `User` GROUP BY City --[x]


-- INNER JOIN (or simply JOIN)
-- Returns only the rows where there is a match in both tables based on the join condition.
-- If no match is found in either table, the row is excluded.
SELECT User.Username, User.Age, User.City, Orders.OrderAmount
FROM `User`
JOIN Orders ON User.Username = Orders.Username

-- LEFT JOIN (or LEFT OUTER JOIN)
-- Returns all rows from the left table (`User`) and the matching rows from the right table (`Orders`).
-- If there is no match in the right table, the result will contain NULL for the columns of the right table.
SELECT User.Username, User.Age, User.City, Orders.OrderAmount
FROM `User`
LEFT JOIN Orders ON User.Username = Orders.Username;

-- RIGHT JOIN (or RIGHT OUTER JOIN)
-- Returns all rows from the right table (`Orders`) and the matching rows from the left table (`User`).
-- If there is no match in the left table, the result will contain NULL for the columns of the left table.
SELECT User.Username, User.Age, User.City, Orders.OrderAmount
FROM `User`
RIGHT JOIN Orders ON User.Username = Orders.Username;

-- FULL OUTER JOIN
-- Returns all rows from both the left (`User`) and right (`Orders`) tables.
-- If there is no match in either table, the result will contain NULL for the missing columns in the non-matching table.
-- This join ensures that no data is excluded, even if there is no match in either table.
SELECT User.Username, User.Age, User.City, Orders.OrderAmount
FROM `User`
FULL OUTER JOIN Orders ON User.Username = Orders.Username;


