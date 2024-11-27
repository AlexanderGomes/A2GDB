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
import org.apache.calcite.sql.ddl.SqlKeyConstraint;
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
import java.util.logging.Logger;
import java.io.*;
import java.net.*;

public class QueryPlanner {
  private static Logger logger = Logger.getLogger(QueryPlanner.class.getName());
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
    SqlInsert sqlInsertNode = (SqlInsert) node;
    String tableName = sqlInsertNode.getTargetTable().toString();

    List<String> columnNames = new ArrayList<>();
    if (sqlInsertNode.getTargetColumnList() != null) {
      for (SqlNode columnNode : sqlInsertNode.getTargetColumnList()) {
        columnNames.add(columnNode.toString());
      }
    }

    List<List<String>> rows = new ArrayList<>();
    SqlBasicCall allRowsNode = (SqlBasicCall) sqlInsertNode.getSource();

    for (SqlNode operand : allRowsNode.getOperandList()) {
      SqlBasicCall singleRowNode = (SqlBasicCall) operand;

      List<String> row = new ArrayList<>();
      for (SqlNode rowValue : singleRowNode.getOperandList()) {
        row.add(rowValue.toString());
      }
      rows.add(row);
    }

    JSONObject jsonBuilder = new JSONObject();
    JSONArray jsonRows = new JSONArray(rows);
    JSONArray jsonSelectedCols = new JSONArray(columnNames);

    jsonBuilder.put("relOp", "INSERT");
    jsonBuilder.put("table", tableName);
    jsonBuilder.put("rows", jsonRows);
    jsonBuilder.put("selectedCols", jsonSelectedCols);

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

    for (Pair<String, Object> entry : refEntries) {
      jWriter.item(entry.left, entry.right);
    }

    jWriter.done(root);

    return jWriter.asString();
  }

  private String handleCreate(SqlNode node) {
    List<Pair<String, String>> columnsInfo = new ArrayList<Pair<String, String>>();

    SqlCreateTable createTableNode = (SqlCreateTable) node;
    SqlIdentifier tableName = createTableNode.name;

    List<SqlNode> columnNodeList = createTableNode.columnList.getList();
    for (SqlNode columnNode : columnNodeList) {
      if (columnNode instanceof SqlColumnDeclaration) {
        SqlColumnDeclaration columnInfo = (SqlColumnDeclaration) columnNode;

        String colName = columnInfo.name.getSimple();
        String colType = columnInfo.dataType.getTypeName().toString();
        Pair<String, String> columnPair = Pair.of(colName, colType);

        columnsInfo.add(columnPair);
      } else if (columnNode instanceof SqlKeyConstraint) {
        SqlKeyConstraint primaryKeyNode = (SqlKeyConstraint) columnNode;
        List<SqlNode> primaryKeyList = primaryKeyNode.getOperandList();

        for (SqlNode primaryKey : primaryKeyList) {
          if (primaryKey != null) {
            Pair<String, String> pair = Pair.of(primaryKey.toString(), "PRIMARY");
            columnsInfo.add(pair);
          }
        }
      }
    }

    addSchemaInMemory(tableName.getSimple(), columnsInfo);
    return encodeCreateTableSchema(tableName.getSimple(), columnsInfo);
  }

  private List<Pair<String, Object>> setSchemas(List<String> tableNames) {
    List<Pair<String, Object>> referenceList = new ArrayList<>();
    int availableIndex = 0;

    for (String tableName : tableNames) {
      Set<String> set = rootSchema.getTableNames();
      if (!set.contains(tableName)) {
        List<Pair<String, String>> columns = getSchemaService(tableName); // fake service
        addSchemaInMemory(tableName, columns);
        availableIndex = ResolveReference(columns, referenceList, availableIndex);
      }
    }

    return referenceList;
  }

  private int ResolveReference(List<Pair<String, String>> columns, List<Pair<String, Object>> refList, int avlIndex) {
    for (Pair<String, String> col : columns) {
      refList.add(Pair.of(((Integer) avlIndex).toString(), col.left));
      avlIndex++;
    }
    return avlIndex;
  }

  private void addSchemaInMemory(String tableName, List<Pair<String, String>> columnsInfo) {
    rootSchema.add(tableName, new AbstractTable() {
      @Override
      public RelDataType getRowType(RelDataTypeFactory typeFactory) {
        RelDataTypeFactory.Builder builder = typeFactory.builder();

        for (Pair<String, String> pair : columnsInfo) {
          builder.add(pair.left,
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
  private List<Pair<String, String>> getSchemaService(String tableName) {
    List<Pair<String, String>> columns = new ArrayList<>();
    if (tableName.equals("Departments")) {
      columns.add(Pair.of("DepartmentID", "VARCHAR"));
      columns.add(Pair.of("DepartmentName", "VARCHAR"));
    } else if (tableName.equals("Employees")) {
      columns.add(Pair.of("DepartmentID", "VARCHAR"));
      columns.add(Pair.of("Name", "VARCHAR"));
    } else if (tableName.equals("kid")) {
      columns.add(Pair.of("dad", "VARCHAR"));
      columns.add(Pair.of("mom", "VARCHAR"));
    } else if (tableName.equals("User")) {
      columns.add(Pair.of("city", "VARCHAR"));
      columns.add(Pair.of("age", "VARCHAR"));
    }
    return columns;
  }

  private String encodeCreateTableSchema(String tableName, List<Pair<String, String>> columnsInfo) {
    JSONObject jsonObj = new JSONObject();
    JSONArray columnsArray = new JSONArray();

    jsonObj.put("relOp", "CREATE_TABLE");
    jsonObj.put("table", tableName);

    for (Pair<String, String> pair : columnsInfo) {
      JSONObject tempJsonObject = new JSONObject();
      tempJsonObject.put(pair.left, pair.right);
      columnsArray.put(tempJsonObject);
    }

    jsonObj.put("columns", columnsArray);
    return jsonObj.toString();
  }

  public static void main(String[] args)
      throws IOException, SQLException, ValidationException, RelConversionException, SqlParseException, Exception {
    QueryPlanner queryPlanner = QueryPlanner.getInstance();

    int port = 8080;
    try (ServerSocket serverSocket = new ServerSocket(port)) {
      logger.info("Server is listening on: " + port);

      while (true) {
        Socket socket = serverSocket.accept();
        logger.info("New client connected");

        new ClientHandler(socket, queryPlanner).start();
      }
    } catch (IOException e) {
      System.out.println("Server Initialization Failure: " + e.getMessage());
      e.printStackTrace();
    }
  }
}

class ClientHandler extends Thread {
  private final Logger logger;
  private final Socket socket;
  private final QueryPlanner planner;

  public ClientHandler(Socket socket, QueryPlanner planner) {
    this.socket = socket;
    this.planner = planner;
    this.logger = Logger.getLogger(ClientHandler.class.getName());
  }

  public void run() {
    try (
        InputStream input = socket.getInputStream();
        BufferedReader reader = new BufferedReader(new InputStreamReader(input));
        OutputStream output = socket.getOutputStream();
        PrintWriter writer = new PrintWriter(output, true)) {

      String query = reader.readLine();
      logger.info("Query Received");

      String encodedPlan = planner.getLogicalPlan(query);
      if (encodedPlan != "") {
        logger.info("Encoded Plan Success");
      }

      writer.print(encodedPlan);

    } catch (IOException e) {
      System.out.println("Server exception: " + e.getMessage());
      e.printStackTrace();
    }
  }
}