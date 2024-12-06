## PSA
Apache Calcite is only being used for parsing and planning.

### Apache Calcite Lessons
- When trying to get a query plan, all stages need to be performed by the planner, such as parsing, validation, etc, otherwise the internal machine state won't be updated and the function will throw an exeption.
- Calcite is more about data manipulation then DDL operations, to be able to create a table I had to import calcite-server which provides SqlDdlParserImpl.FACTORY configuration for the parser.
- The planner needs to know the schema of all tables when creating a query plan, so before validating we're taking care of it by reaching our for a fake metadata service.


## Schema Persistence
- When a table is created it gets added to apache calcite which stores it in memory, so if you run another query referencing that table it will work, but if the process gets terminated and you run a query referencing that table it will fail because its not in memory, it needs to be fetched from disk.