INSERT INTO `User` (Username, Age, City) VALUES ('JaneSmith', 25, 'Los Angeles'), ('AliceBrown', 28, 'Chicago'), ('BobWhite', 35, 'Houston')
CREATE TABLE `User`(PRIMARY KEY(UserId), Username VARCHAR, Age INT, City VARCHAR) [x]
SELECT * FROM `User` [x]
SELECT Username, Age FROM `User` [x]
SELECT Username, Age, City FROM `User` WHERE Age > 20 [x]
SELECT Username, City FROM `User` WHERE UserId = CAST('10084632547061476038' AS DECIMAL(20,0)) [x]
SELECT Username, Age FROM `User` WHERE City = 'Los Angeles' [x]
SELECT Username, Age, City FROM `User` ORDER BY Age ASC [x]
SELECT Username, Age, City FROM `User` ORDER BY Age DESC [x]
SELECT Username, Age, City FROM `User` ORDER BY Age ASC LIMIT 1 [x]
SELECT Username, Age, City FROM `User` ORDER BY Age DESC LIMIT 1 [x]
SELECT Username, Age, City FROM `User` WHERE Age BETWEEN 20 AND 30 [x]


SELECT City, COUNT(*) AS UserCount FROM `User` GROUP BY City [x]
SELECT City, MAX(Age) AS max_age FROM `User` GROUP BY City [x]
SELECT City, MIN(Age) AS max_age FROM `User` GROUP BY City [x]
SELECT City, AVG(Age) AS max_age FROM `User` GROUP BY City [x]
SELECT City, SUM(Age) AS max_age FROM `User` GROUP BY City [x]




// EXTRAS
SELECT * FROM `User` WHERE City = 'Los Angeles' AND Age > 25 []
SELECT * FROM `User` WHERE City = 'Houston' OR Age < 30 []
SELECT * FROM `User` WHERE City IN ('Los Angeles', 'Chicago') []








CREATE TABLE Orders (
    PRIMARY KEY(OrderID),
    Username VARCHAR(50),
    OrderAmount DECIMAL
);

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

CREATE TABLE Products (
    ProductID INT PRIMARY KEY,
    ProductName VARCHAR(100)
);


SELECT User.Username, Orders.OrderAmount, Products.ProductName
FROM `User`
INNER JOIN Orders ON User.Username = Orders.Username
INNER JOIN Products ON Orders.OrderID = Products.ProductID;
