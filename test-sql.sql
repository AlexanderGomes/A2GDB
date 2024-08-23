INSERT INTO User (UserID, Username, PasswordHash) VALUES
			(1, 'sander', 'hashed_password_1'),
			(2, 'john_doe', 'hashed_password_1'),
			(3, 'john_doe', 'hashed_password_1'),
			(4, 'john_doe', 'hashed_password_1'),
			(5, 'john_doe', 'hashed_password_1');


UPDATE User
SET UserID = 292992992
WHERE UserID = 1;


CREATE TABLE User (
			UserID INT PRIMARY KEY,
			Username VARCHAR,
			PasswordHash VARCHAR
);

DELETE FROM User WHERE Username = 'john_doe';