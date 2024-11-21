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

import java.sql.SQLException;
import java.util.ArrayList;
import java.util.List;
import java.util.Set;

import java.io.*;
import java.net.*;

public class QueryPlanner {
  private static QueryPlanner instance;
  private final Planner planner;
  private final SchemaPlus rootSchema;

  private QueryPlanner() {
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

  public static QueryPlanner getInstance() {
    if (instance == null) {
      synchronized (QueryPlanner.class) {
        if (instance == null) {
          instance = new QueryPlanner();
        }
      }
    }
    return instance;
  }

  public String getLogicalPlan(String query) {
    String jsonPlan = "";

    try {
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
    } catch (Exception e) {
      System.err.println("Couldn't Create Query Plan: " + e.getMessage());
      e.printStackTrace();
    }
    return jsonPlan;
  }

  private String handleInsert(SqlNode node) {
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

  private String handleSelect(SqlNode node) throws ValidationException, RelConversionException {
    List<String> tableNames = GetTableName(node);
    List<Pair<String, Object>> refEntries = setSchemas(tableNames);

    SqlNode validatedSqlNode = planner.validate(node);
    RelNode root = planner.rel(validatedSqlNode).project();

    JsonBuilder jBuilder = new JsonBuilder();
    RelJsonWriter jWriter = new RelJsonWriter(jBuilder);

    jWriter.item("relOp", "references");
    System.out.println(refEntries);

    for (Pair<String, Object> entry : refEntries) {
      jWriter.item(entry.left, entry.right);
    }

    jWriter.done(root);

    return jWriter.asString();
  }

  private String handleCreate(SqlNode node) {
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
    QueryPlanner queryPlanner = QueryPlanner.getInstance();

    int port = 8080;
    try (ServerSocket serverSocket = new ServerSocket(port)) {
      System.out.println("Server is listening on: " + port);

      while (true) {
        Socket socket = serverSocket.accept();
        System.out.println("New client connected");

        new ClientHandler(socket, queryPlanner).start();
      }
    } catch (IOException e) {
      System.out.println("Server Initialization Failure: " + e.getMessage());
      e.printStackTrace();
    }
  }
}

class ClientHandler extends Thread {
  private Socket socket;
  private QueryPlanner planner;

  public ClientHandler(Socket socket, QueryPlanner planner) {
    this.socket = socket;
    this.planner = planner;
  }

  public void run() {
    try (
        InputStream input = socket.getInputStream();
        BufferedReader reader = new BufferedReader(new InputStreamReader(input));
        OutputStream output = socket.getOutputStream();
        PrintWriter writer = new PrintWriter(output, true)) {

      String query = reader.readLine();
      System.out.println("Query received: " + query);

      String encodedPlan = planner.getLogicalPlan(query);
      writer.print(encodedPlan);

    } catch (IOException e) {
      System.out.println("Server exception: " + e.getMessage());
      e.printStackTrace();
    }
  }
}