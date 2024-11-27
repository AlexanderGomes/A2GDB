INSERT INTO `User` (Username, Age, City) VALUES ('sander0909', 18, "Richmind")


UPDATE User SET UserID = 292992992 WHERE UserID = 1;

CREATE TABLE `User`(PRIMARY KEY(UserId), Username VARCHAR, Age INT, City VARCHAR)

CREATE TABLE `Student` (
			UserID INT,
			Username CHAR,
			PasswordHash CHAR
);

DELETE FROM User WHERE Username = 'john_doe';

SELECT * 
FROM User 
JOIN Student ON Student.Username = User.Username;

SELECT Employees.Name, Departments.DepartmentName
FROM Employees
JOIN Departments ON Employees.DepartmentID = Departments.DepartmentID AND Departments.DepartmentID = 1828128


SELECT city, AVG(age) as average_age
FROM User
GROUP BY city;


				  RESULT
			        |
					|
                    |
				PROJECTION (e.employee_id, e.name, d.department_name, s.salary)
					|
                    |
				    |
				   JOIN
         --------------------------
           /                     \
          /                        \
         /                          \ (PREDICATE e.employee_id = s.employee_id) (optional)
        JOIN                        /
    /          \                   /
   /            \                 /
  /              \               /
get employee  get department    get salaries