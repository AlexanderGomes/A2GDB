package planner;

import org.apache.calcite.config.Lex;
import org.apache.calcite.plan.Contexts;
import org.apache.calcite.rel.RelNode;
import org.apache.calcite.rel.type.RelDataType;
import org.apache.calcite.rel.type.RelDataTypeFactory;
import org.apache.calcite.rel.type.RelDataTypeSystem;
import org.apache.calcite.schema.SchemaPlus;
import org.apache.calcite.schema.impl.AbstractTable;
import org.apache.calcite.sql.SqlBasicCall;
import org.apache.calcite.sql.SqlIdentifier;
import org.apache.calcite.sql.SqlInsert;
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
import org.apache.calcite.util.JsonBuilder;
import org.apache.calcite.util.Pair;
import org.json.JSONArray;
import org.json.JSONObject;
import org.apache.calcite.rel.externalize.RelJsonWriter;

import java.io.IOException;
import java.sql.SQLException;
import java.util.ArrayList;
import java.util.List;
import java.util.Set;

public class QueryPlanner {
  private final Planner planner;
  private final SchemaPlus rootSchema;

  public QueryPlanner() {
    Config parserConfig = SqlParser.config()
        .withLex(Lex.MYSQL)
        .withParserFactory(SqlDdlParserImpl.FACTORY)
        .withConformance(SqlConformanceEnum.LENIENT);

    this.rootSchema = Frameworks.createRootSchema(true);
    FrameworkConfig calciteFrameworkConfig = Frameworks.newConfigBuilder()
        .parserConfig(parserConfig)
        .defaultSchema(rootSchema)
        .context(Contexts.EMPTY_CONTEXT)
        .costFactory(null)
        .typeSystem(RelDataTypeSystem.DEFAULT)
        .build();

    this.planner = Frameworks.getPlanner(calciteFrameworkConfig);
  }

  private String getLogicalPlan(String query)
      throws ValidationException, RelConversionException, SqlParseException, Exception {
    String jsonPlan = "";
    SqlNode sqlNode = planner.parse(query);

    if (sqlNode instanceof SqlCreateTable) {
      jsonPlan = handleCreate(sqlNode);
    } else if (sqlNode instanceof SqlSelect) {
      jsonPlan = handleSelect(sqlNode);
    } else if (sqlNode instanceof SqlInsert) {
      jsonPlan = handleInsert(sqlNode);
    } else {
      throw new Exception("sqlNode type unhandled");
    }

    planner.close();
    return jsonPlan;
  }

  public String handleInsert(SqlNode node) {
    SqlInsert sqlInsert = (SqlInsert) node;
    String tableName = sqlInsert.getTargetTable().toString();

    List<String> columnNames = new ArrayList<>();
    if (sqlInsert.getTargetColumnList() != null) {
      for (SqlNode columnNode : sqlInsert.getTargetColumnList()) {
        columnNames.add(columnNode.toString());
      }
    }

    List<List<String>> rows = new ArrayList<>();
    SqlNode source = sqlInsert.getSource();
    if (source instanceof SqlBasicCall) {
      SqlBasicCall basicCall = (SqlBasicCall) source;
      for (SqlNode operand : basicCall.getOperandList())
        if (operand instanceof SqlBasicCall) {

          SqlBasicCall rowCall = (SqlBasicCall) operand;
          List<String> row = new ArrayList<>();
          for (SqlNode value : rowCall.getOperandList()) {
            row.add(value.toString());
          }
          rows.add(row);
        }
    }

    JSONObject jsonBuilder = new JSONObject();
    JSONArray jsonRows = new JSONArray(rows);

    jsonBuilder.put("relOp", "INSERT");
    jsonBuilder.put("tableName", tableName);
    jsonBuilder.put("rows", jsonRows);

    return jsonBuilder.toString();
  }

  public String handleSelect(SqlNode node) throws ValidationException, RelConversionException {
    List<String> tableNames = GetTableName(node);
    List<Pair<String, Object>> refEntries = setSchemas(tableNames);

    SqlNode validatedSqlNode = planner.validate(node);
    RelNode root = planner.rel(validatedSqlNode).project();

    JsonBuilder jBuilder = new JsonBuilder();
    RelJsonWriter jWriter = new RelJsonWriter(jBuilder);

    jWriter.item("relOp", "references");
    for (Pair<String, Object> entry : refEntries) {
      jWriter.item(entry.left, entry.right);
    }

    jWriter.done(root);

    return jWriter.asString();
  }

  public String handleCreate(SqlNode node) {
    List<String> columns = new ArrayList<String>();
    SqlCreateTable createTable = (SqlCreateTable) node;
    SqlIdentifier tableName = createTable.name;

    List<SqlNode> columnsNodes = createTable.columnList.getList();
    for (SqlNode columnNode : columnsNodes) {
      if (columnNode instanceof SqlColumnDeclaration) {
        SqlColumnDeclaration column = (SqlColumnDeclaration) columnNode;
        columns.add(column.name.getSimple());
      }
    }
    addTableSchema(tableName.getSimple(), columns);
    return createTable(tableName.getSimple(), columns);
  }

  private List<Pair<String, Object>> setSchemas(List<String> tableNames) {
    List<Pair<String, Object>> refList = new ArrayList<>();
    int availableIndex = 0;

    for (String tableName : tableNames) {
      Set<String> set = rootSchema.getTableNames();
      if (!set.contains(tableName)) {
        List<String> columns = getSchemaService(tableName);
        addTableSchema(tableName, columns);
        availableIndex = ResolveReference(columns, refList, availableIndex);
      }
    }

    return refList;
  }

  private int ResolveReference(List<String> columns, List<Pair<String, Object>> refList, int avlIndex) {
    for (String col : columns) {
      refList.add(Pair.of(((Integer) avlIndex).toString(), col));
      avlIndex++;
    }
    return avlIndex;
  }

  private void addTableSchema(String tableName, List<String> columns) {
    rootSchema.add(tableName, new AbstractTable() {
      @Override
      public RelDataType getRowType(RelDataTypeFactory typeFactory) {
        RelDataTypeFactory.Builder builder = typeFactory.builder();

        for (String column : columns) {
          builder.add(column,
              typeFactory.createTypeWithNullability(typeFactory.createSqlType(SqlTypeName.VARCHAR), true));
        }

        return builder.build();
      }
    });
  }

  private List<String> GetTableName(SqlNode sqlNode) {
    SqlIdentifier table;
    List<String> tables = new ArrayList<String>();

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
        table = (SqlIdentifier) leftTable;
        tables.add(table.getSimple());
      }

      if (rightTable instanceof SqlIdentifier) {
        table = (SqlIdentifier) rightTable;
        tables.add(table.getSimple());
      }
    }

    return tables;
  }

  // service to be implemented
  // get columns over the wire
  private List<String> getSchemaService(String tableName) {
    List<String> columns = new ArrayList<>();
    if (tableName.equals("Departments")) {
      columns.add("DepartmentID");
      columns.add("DepartmentName");
    } else if (tableName.equals("Employees")) {
      columns.add("DepartmentID");
      columns.add("Name");
    } else if (tableName.equals("kid")) {
      columns.add("dad");
      columns.add("mom");
    } else if (tableName.equals("User")) {
      columns.add("city");
      columns.add("age");
    }
    return columns;
  }

  private String createTable(String tableName, List<String> columns) {
    JSONObject jsonBuilder = new JSONObject();
    JSONArray columnsArray = new JSONArray(columns);
    jsonBuilder.put("relOp", "CREATE_TABLE");
    jsonBuilder.put("tableName", tableName);
    jsonBuilder.put("columns", columnsArray);
    return jsonBuilder.toString();
  }

  public static void main(String[] args)
      throws IOException, SQLException, ValidationException, RelConversionException, SqlParseException, Exception {

    QueryPlanner queryPlanner = new QueryPlanner();

    String jsonPlan1 = queryPlanner
        .getLogicalPlan("INSERT INTO Users (id, name) VALUES (1, 'Alice'), (2, 'Bob'), (3, 'Charlie')");
    System.out.println(jsonPlan1);
  }
}
