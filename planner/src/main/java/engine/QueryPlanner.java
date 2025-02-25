package engine;

import org.apache.calcite.config.Lex;
import org.apache.calcite.plan.Contexts;
import org.apache.calcite.rel.RelNode;
import org.apache.calcite.rel.core.AggregateCall;
import org.apache.calcite.rel.type.RelDataType;
import org.apache.calcite.rel.type.RelDataTypeFactory;
import org.apache.calcite.rel.type.RelDataTypeSystem;
import org.apache.calcite.schema.SchemaPlus;
import org.apache.calcite.schema.impl.AbstractTable;
import org.apache.calcite.sql.SqlBasicCall;
import org.apache.calcite.sql.SqlCall;
import org.apache.calcite.sql.SqlDelete;
import org.apache.calcite.sql.SqlIdentifier;
import org.apache.calcite.sql.SqlInsert;
import org.apache.calcite.sql.SqlJoin;
import org.apache.calcite.sql.SqlNode;
import org.apache.calcite.sql.SqlNodeList;
import org.apache.calcite.sql.SqlOrderBy;
import org.apache.calcite.sql.SqlSelect;
import org.apache.calcite.sql.SqlUpdate;
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
import org.apache.calcite.rel.logical.LogicalAggregate;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.fasterxml.jackson.databind.node.ArrayNode;
import com.fasterxml.jackson.databind.node.ObjectNode;
import com.fasterxml.jackson.core.JsonProcessingException;

import java.sql.SQLException;
import java.util.ArrayList;
import java.util.List;
import java.util.Set;
import java.util.logging.Logger;
import java.io.*;
import java.net.*;
import java.util.HashMap;

public class QueryPlanner {
  private static Logger logger = Logger.getLogger(QueryPlanner.class.getName());
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

  public static QueryPlanner create() {
    return new QueryPlanner();
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
      } else if (sqlNode instanceof SqlOrderBy) {
        jsonPlan = handleOrderBy(sqlNode);
      } else if (sqlNode instanceof SqlDelete) {
        jsonPlan = handleDelete(sqlNode);
      } else if (sqlNode instanceof SqlUpdate) {
        jsonPlan = handleUpdate(sqlNode);
      } else {
        throw new Exception("sqlNode type unhandled");
      }

    } catch (Exception e) {
      planner.close();
      jsonPlan = handleExepction(e);
    }

    planner.close();
    return jsonPlan;
  }

  private String handleExepction(Exception e) {
    JSONObject errorResponse = new JSONObject();

    errorResponse.put("status", "error");
    errorResponse.put("message", e.getMessage());
    errorResponse.put("errorType", e.getClass().getSimpleName());

    return errorResponse.toString();
  }

  private String handleUpdate(SqlNode node) {
    SqlUpdate updateNode = (SqlUpdate) node;

    SqlNode updateCondition = updateNode.getCondition();
    SqlBasicCall condition = (SqlBasicCall) updateCondition;

    String leftOperand = condition.getOperandList().get(0).toString();
    String rightOperand = condition.getOperandList().get(1).toString();

    String tableName = updateNode.getTargetTable().toString();

    String targetColumn = updateNode.getTargetColumnList().get(0).toString();
    String sourceExpression = updateNode.getSourceExpressionList().get(0).toString();

    JSONObject jsonObj = new JSONObject();
    jsonObj.put("STATEMENT", "UPDATE");
    jsonObj.put("table", tableName);

    jsonObj.put("modify_column", targetColumn);
    jsonObj.put("modify_value", sourceExpression);

    jsonObj.put("filter_column", leftOperand);
    jsonObj.put("filter_value", rightOperand);

    return jsonObj.toString();
  }

  private String handleDelete(SqlNode node) {
    JSONObject jsonObj = new JSONObject();

    SqlDelete deleteNode = (SqlDelete) node;
    String tableName = deleteNode.getTargetTable().toString();

    SqlNode whereCondition = deleteNode.getCondition();
    if (whereCondition != null) {
      if (whereCondition instanceof SqlCall) {
        SqlCall whereCall = (SqlCall) whereCondition;
        if (whereCall.getOperator().getName().equals("=")) {
          SqlNode leftOperand = whereCall.operand(0);
          SqlNode rightOperand = whereCall.operand(1);

          String columnName = leftOperand.toString();
          String value = rightOperand.toString();

          jsonObj.put("STATEMENT", "DELETE");
          jsonObj.put("table", tableName);
          jsonObj.put("column", columnName);
          jsonObj.put("value", value);
        }
      }
    }

    return jsonObj.toString();
  }

  private String handleOrderBy(SqlNode node)
      throws ValidationException, RelConversionException, JsonProcessingException {
    String jsonPlan = "";
    String sortDirection = "";
    String column = "";
    boolean isDesc = false;
    String fetchVal = "";

    SqlOrderBy orderByNode = (SqlOrderBy) node;
    fetchVal = orderByNode.fetch != null ? orderByNode.fetch.toString() : fetchVal;

    SqlNode query = orderByNode.query;
    jsonPlan = handleSelect(query);

    SqlNodeList orderList = orderByNode.orderList;

    for (SqlNode order : orderList) {
      if (order instanceof SqlBasicCall) {
        SqlBasicCall sqlBasicCall = (SqlBasicCall) order;

        List<SqlNode> operands = sqlBasicCall.getOperandList();
        column = operands.get(0).toString();
        isDesc = true;
      }
    }

    sortDirection = isDesc ? "DESC" : "ASC";
    column = !isDesc ? orderList.toString().replace("`", "") : column;

    ObjectMapper objectMapper = new ObjectMapper();
    JsonNode rootNode = objectMapper.readTree(jsonPlan);
    ArrayNode relsArray = (ArrayNode) rootNode.path("rels");

    ObjectNode newRelObject = objectMapper.createObjectNode();
    newRelObject.put("relOp", "LogicalSort");
    newRelObject.put("sortDirection", sortDirection);
    newRelObject.put("column", column);
    newRelObject.put("limit", fetchVal);

    relsArray.add(newRelObject);

    jsonPlan = objectMapper.writeValueAsString(rootNode);

    return jsonPlan;
  }

  private String handleInsert(SqlNode node) {
    SqlInsert insertNode = (SqlInsert) node;
    String tableName = insertNode.getTargetTable().toString();

    List<String> columnNames = new ArrayList<>();
    if (insertNode.getTargetColumnList() != null) {
      for (SqlNode columnNode : insertNode.getTargetColumnList()) {
        columnNames.add(columnNode.toString());
      }
    }

    List<List<String>> rows = new ArrayList<>();
    SqlBasicCall allRowsNode = (SqlBasicCall) insertNode.getSource();

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

    jsonBuilder.put("STATEMENT", "INSERT");
    jsonBuilder.put("table", tableName);
    jsonBuilder.put("rows", jsonRows);
    jsonBuilder.put("selectedCols", jsonSelectedCols);

    return jsonBuilder.toString();
  }

  private String handleSelect(SqlNode node)
      throws ValidationException, RelConversionException, JsonProcessingException {
    List<String> tableNames = GetTableName(node);
    HashMap<String, String> refEntries = setSchemas(tableNames);

    SqlNode validatedSqlNode = planner.validate(node);
    RelNode root = planner.rel(validatedSqlNode).project();

    JsonBuilder jBuilder = new JsonBuilder();
    RelJsonWriter jWriter = new RelJsonWriter(jBuilder);

    List<String> columnNames = root.getRowType().getFieldNames();
    jWriter.item("selected_columns", columnNames);
    jWriter.done(root);

    String initialJsonString = jWriter.asString();

    ObjectMapper mapper = new ObjectMapper();
    String refEntriesJsonString = mapper.writeValueAsString(refEntries);
    JSONObject finalJson = new JSONObject(initialJsonString);

    finalJson.put("STATEMENT", "SELECT");
    finalJson.put("refList", new JSONObject(refEntriesJsonString));

    String jsonResponse = finalJson.toString();

    if (root instanceof LogicalAggregate) {
      LogicalAggregate aggregateNode = (LogicalAggregate) root;

      List<AggregateCall> aggCalls = aggregateNode.getAggCallList();
      for (AggregateCall call : aggCalls) {
        String functionName = call.getAggregation().getName();
        List<Integer> functionArgs = call.getArgList();

        JsonNode rootNode = mapper.readTree(jsonResponse);
        ArrayNode relsArray = (ArrayNode) rootNode.path("rels");

        for (JsonNode rel : relsArray) {
          if (rel.path("relOp").asText().equals("LogicalAggregate")) {
            ObjectNode aggNode = (ObjectNode) rel;
            ObjectNode aggregates = mapper.createObjectNode();

            aggregates.put("function", functionName);

            ArrayNode argsArray = mapper.createArrayNode();
            for (Integer arg : functionArgs) {
              argsArray.add(arg);
            }

            aggregates.set("args", argsArray);

            aggNode.set("aggregates", aggregates);
            jsonResponse = mapper.writeValueAsString(rootNode);
          }
        }
      }
    }

    return jsonResponse;
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
            String cleanedKey = primaryKey.toString().replace("`", "");
            Pair<String, String> pair = Pair.of(cleanedKey, "PRIMARY");
            columnsInfo.add(pair);
          }
        }
      }
    }

    addSchemaInMemory(tableName.getSimple(), columnsInfo);
    return encodeCreateTableSchema(tableName.getSimple(), columnsInfo);
  }

  private HashMap<String, String> setSchemas(List<String> tableNames) {
    HashMap<String, String> refList = new HashMap<String, String>();
    int availableIndex = 0;

    for (String tableName : tableNames) {
      Set<String> set = rootSchema.getTableNames();
      List<Pair<String, String>> columns = getSchema(tableName);
      if (!set.contains(tableName)) {
        addSchemaInMemory(tableName, columns);
      }
      availableIndex = ResolveReference(columns, refList, availableIndex);
    }
    return refList;
  }

  private int ResolveReference(List<Pair<String, String>> columns, HashMap<String, String> refList, int avlIndex) {
    for (Pair<String, String> col : columns) {
      refList.put(String.valueOf("$" + avlIndex), col.left);
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
              // #### changing the type from VARCHAR to ANY causes a different query plan
              // which breaks your backend
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

  private List<Pair<String, String>> getSchema(String tableName) {
    List<Pair<String, String>> columns = new ArrayList<>();

    String jsonColumns = DbSchemas.get(tableName);
    if (jsonColumns == null) {
      throw new IllegalArgumentException("Table schema not found for: " + tableName);
    }

    JSONArray columnsArray = new JSONArray(jsonColumns);

    for (int i = 0; i < columnsArray.length(); i++) {
      JSONObject column = columnsArray.getJSONObject(i);
      String key = column.keys().next();
      String value = column.getString(key);
      columns.add(Pair.of(key, value));
    }

    return columns;
  }

  private String encodeCreateTableSchema(String tableName, List<Pair<String, String>> columnsInfo) {
    JSONObject jsonObj = new JSONObject();
    JSONArray columnsArray = new JSONArray();

    jsonObj.put("STATEMENT", "CREATE_TABLE");
    jsonObj.put("table", tableName);

    for (Pair<String, String> pair : columnsInfo) {
      JSONObject tempJsonObject = new JSONObject();
      tempJsonObject.put(pair.left, pair.right);
      columnsArray.put(tempJsonObject);
    }

    DbSchemas.put(tableName, columnsArray.toString());

    jsonObj.put("columns", columnsArray);

    return jsonObj.toString();
  }

  public static void main(String[] args)
      throws IOException, SQLException, ValidationException, RelConversionException, SqlParseException, Exception {

    int port = 8080;
    try (ServerSocket serverSocket = new ServerSocket(port)) {
      logger.info("Server is listening on: " + port);

      while (true) {
        Socket socket = serverSocket.accept();
        logger.info("New client connected");
        QueryPlanner queryPlanner = QueryPlanner.create();
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
