package planner;

import org.apache.calcite.config.Lex;
import org.apache.calcite.plan.Contexts;
import org.apache.calcite.plan.RelOptUtil;
import org.apache.calcite.rel.RelNode;
import org.apache.calcite.rel.type.RelDataType;
import org.apache.calcite.rel.type.RelDataTypeFactory;
import org.apache.calcite.rel.type.RelDataTypeSystem;
import org.apache.calcite.schema.SchemaPlus;
import org.apache.calcite.schema.impl.AbstractTable;
import org.apache.calcite.sql.SqlIdentifier;
import org.apache.calcite.sql.SqlJoin;
import org.apache.calcite.sql.SqlNode;
import org.apache.calcite.sql.SqlSelect;
import org.apache.calcite.sql.ddl.SqlColumnDeclaration;
import org.apache.calcite.sql.ddl.SqlCreateTable;
import org.apache.calcite.sql.parser.SqlParseException;
import org.apache.calcite.sql.parser.SqlParser;
import org.apache.calcite.sql.parser.SqlParser.Config;
import org.apache.calcite.sql.parser.ddl.SqlDdlParserImpl;
import org.apache.calcite.sql.type.SqlTypeName;
import org.apache.calcite.sql.validate.SqlConformanceEnum;
import org.apache.calcite.tools.*;

import java.io.IOException;
import java.sql.SQLException;
import java.util.ArrayList;
import java.util.List;
import java.util.Set;

public class QueryPlanner {
  private final Planner planner;
  private final SchemaPlus rootSchema;

  public QueryPlanner() {
    this.rootSchema = Frameworks.createRootSchema(true);
    Config parserConfig = SqlParser.config().withLex(Lex.MYSQL).withParserFactory(SqlDdlParserImpl.FACTORY)
        .withConformance(SqlConformanceEnum.LENIENT);
    FrameworkConfig calciteFrameworkConfig = Frameworks.newConfigBuilder()
        .parserConfig(parserConfig)
        .defaultSchema(rootSchema)
        .context(Contexts.EMPTY_CONTEXT)
        .costFactory(null)
        .typeSystem(RelDataTypeSystem.DEFAULT)
        .build();

    this.planner = Frameworks.getPlanner(calciteFrameworkConfig);
  }

  private void getLogicalPlan(String query)
      throws ValidationException, RelConversionException, SqlParseException, Exception {
    SqlNode sqlNode = planner.parse(query);

    if (sqlNode instanceof SqlCreateTable) {
      List<String> columns = new ArrayList<String>();
      SqlCreateTable createTable = (SqlCreateTable) sqlNode;
      SqlIdentifier tableName = createTable.name;

      List<SqlNode> columnsNodes = createTable.columnList.getList();
      for (SqlNode columnNode : columnsNodes) {
        if (columnNode instanceof SqlColumnDeclaration) {
          SqlColumnDeclaration column = (SqlColumnDeclaration) columnNode;
          columns.add(column.name.getSimple());
        }
      }
      addTableSchema(tableName.getSimple(), columns);
      createTable(tableName.getSimple(), columns);
      return;
    }

    List<String> tableNames = GetTableName(sqlNode);
    setSchemas(tableNames);

    SqlNode validatedSqlNode = planner.validate(sqlNode);
    RelNode root = planner.rel(validatedSqlNode).project();

    planner.close();

    System.out.println(RelOptUtil.toString(root));
    // encode query plan and send it over the wire.
  }

  private void createTable(String name, List<String> columns) {
    // encode and send to the metadata service or storage engine
  }

  private void setSchemas(List<String> tableNames) {
    for (String name : tableNames) {
      Set<String> set = rootSchema.getTableNames();
      if (!set.contains(name)) {
        List<String> columns = getSchema(name);
        addTableSchema(name, columns);
      }
    }
  }

  private void addTableSchema(String name, List<String> columns) {
    rootSchema.add(name, new AbstractTable() {
      @Override
      public RelDataType getRowType(RelDataTypeFactory typeFactory) {
        RelDataTypeFactory.Builder builder = typeFactory.builder();

        for (String item : columns) {
          builder.add(item,
              typeFactory.createTypeWithNullability(typeFactory.createSqlType(SqlTypeName.VARCHAR), true));
        }
        return builder.build();
      }
    });
  }

  // service to be implemented
  // get columns over the wire
  private List<String> getSchema(String name) {
    List<String> columns = new ArrayList<>();
    columns.add("DepartmentID");
    columns.add("DepartmentName");
    columns.add("dad");
    columns.add("mom");
    columns.add("Name");
    columns.add("city");
    columns.add("age");
    columns.add("userID");
    return columns;
  }

  private List<String> GetTableName(SqlNode sqlNode) {
    SqlIdentifier table = null;
    SqlIdentifier leftTableName = null;
    SqlIdentifier rightTableName = null;

    List<String> tables = new ArrayList<String>();

    if (sqlNode instanceof SqlSelect) {
      SqlSelect select = (SqlSelect) sqlNode;

      SqlNode fromNode = select.getFrom();

      if (fromNode instanceof SqlIdentifier) {
        table = (SqlIdentifier) fromNode;
        tables.add(table.getSimple());
      } else if (fromNode instanceof SqlJoin) {
        SqlJoin join = (SqlJoin) fromNode;
        SqlNode leftTable = join.getLeft();
        SqlNode rightTable = join.getRight();

        if (leftTable instanceof SqlIdentifier) {
          leftTableName = (SqlIdentifier) leftTable;
          tables.add(leftTableName.getSimple());
        }

        if (rightTable instanceof SqlIdentifier) {
          rightTableName = (SqlIdentifier) rightTable;
          tables.add(rightTableName.getSimple());
        }
      }
    }

    return tables;
  }

  public static void main(String[] args)
      throws IOException, SQLException, ValidationException, RelConversionException, SqlParseException, Exception {

    QueryPlanner queryPlanner = new QueryPlanner();
    // queryPlanner.getLogicalPlan(
    // "CREATE TABLE Student (\n" + //
    // "\t\t\tUserID INT NOT NULL,\n" + //
    // "\t\t\tUsername VARCHAR,\n" + //
    // "\t\t\tPasswordHash VARCHAR\n" + //
    // ")");

    queryPlanner.getLogicalPlan(
        "SELECT Employees.Name, Departments.DepartmentName FROM Employees JOIN Departments ON Employees.DepartmentID = Departments.DepartmentID AND Departments.DepartmentID = 1828128");

    queryPlanner.getLogicalPlan("SELECT dad, mom FROM kid");

    queryPlanner.getLogicalPlan(" SELECT city, AVG(age) as average_age\n" + //
        "FROM `User`\n" + //
        "GROUP BY city");

    queryPlanner.getLogicalPlan("UPDATE `User` SET userID = 292992992 WHERE userID = 1");

  }
}