-- basic queries
[x] INSERT INTO `User` (Username, Age, City) VALUES ('JaneSmith', 25, 'Los Angeles'), ('AliceBrown', 28, 'Chicago'), ('BobWhite', 35, 'Houston')
[x] CREATE TABLE `User`(PRIMARY KEY(UserId), Username VARCHAR, Age INT, City VARCHAR) 
[x] CREATE TABLE Orders (PRIMARY KEY(OrderID),Username VARCHAR(50),OrderAmount DECIMAL) --[x]

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


-- CRUD
[x] DELETE FROM `User` WHERE Username = 'JaneSmith' --[x]
[x] UPDATE `User` SET Age = 121 WHERE Username = 'JaneSmith' --[x]

[] ALTER TABLE `User` ADD COLUMN Email VARCHAR;
[] ALTER TABLE `User` DROP COLUMN Email;


-- EXTRA
[] SELECT * FROM `User` WHERE City = 'Los Angeles' AND Age > 25
[] SELECT * FROM `User` WHERE City = 'Houston' OR Age < 30
[] SELECT * FROM `User` WHERE City IN ('Los Angeles', 'Chicago')
[] SELECT * FROM `User` WHERE Age > (SELECT AVG(Age) FROM `User`)
[] SELECT * FROM `User` ORDER BY Age ASC LIMIT 10 OFFSET 20;
[] SELECT * FROM `User` WHERE Username LIKE 'A%';
[] SELECT City, COUNT(*) AS UserCount  FROM `User` GROUP BY City HAVING COUNT(*) > 1;



--COMPLEX QUERIES

-- subqueries
[] SELECT Username, (SELECT COUNT(*) FROM `Order` o WHERE o.UserId = u.UserId) AS OrderCount FROM `User` u;

-- JOINS
SELECT User.Username, User.Age, User.City, Orders.OrderAmount
FROM `User`
INNER JOIN Orders ON User.Username = Orders.Username;

SELECT User.Username, User.Age, User.City, Orders.OrderAmount
FROM `User`
LEFT JOIN Orders ON User.Username = Orders.Username;

SELECT User.Username, User.Age, User.City, Orders.OrderAmount
FROM `User`
RIGHT JOIN Orders ON User.Username = Orders.Username;

SELECT User.Username, User.Age, User.City, Orders.OrderAmount
FROM `User`
FULL OUTER JOIN Orders ON User.Username = Orders.Username;

SELECT User.Username, User.Age, User.City, Orders.OrderAmount
FROM `User`
INNER JOIN Orders ON User.Username = Orders.Username
WHERE Orders.OrderAmount > 50;

SELECT User.Username, SUM(Orders.OrderAmount) AS TotalSpent
FROM `User`
INNER JOIN Orders ON User.Username = Orders.Username
GROUP BY User.Username;

SELECT User.Username, SUM(Orders.OrderAmount) AS TotalSpent
FROM `User`
INNER JOIN Orders ON User.Username = Orders.Username
GROUP BY User.Username
ORDER BY TotalSpent DESC
LIMIT 1;