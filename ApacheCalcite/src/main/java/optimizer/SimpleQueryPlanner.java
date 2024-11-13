package optimizer;

import org.apache.calcite.config.Lex;
import org.apache.calcite.plan.Contexts;
import org.apache.calcite.plan.RelOptUtil;
import org.apache.calcite.rel.RelNode;
import org.apache.calcite.rel.type.RelDataType;
import org.apache.calcite.rel.type.RelDataTypeFactory;
import org.apache.calcite.rel.type.RelDataTypeSystem;
import org.apache.calcite.schema.SchemaPlus;
import org.apache.calcite.schema.impl.AbstractTable;
import org.apache.calcite.sql.SqlBasicCall;
import org.apache.calcite.sql.SqlIdentifier;
import org.apache.calcite.sql.SqlJoin;
import org.apache.calcite.sql.SqlNode;
import org.apache.calcite.sql.SqlSelect;
import org.apache.calcite.sql.parser.SqlParseException;
import org.apache.calcite.sql.parser.SqlParser;
import org.apache.calcite.sql.type.SqlTypeName;
import org.apache.calcite.tools.*;

import java.io.IOException;
import java.sql.SQLException;
import java.util.ArrayList;
import java.util.HashMap;
import java.util.List;
import java.util.Map;

public class SimpleQueryPlanner {
  private final Planner planner;
  private final SchemaPlus rootSchema;

  public SimpleQueryPlanner() {
    this.rootSchema = Frameworks.createRootSchema(true);
    FrameworkConfig calciteFrameworkConfig = Frameworks.newConfigBuilder()
        .parserConfig(SqlParser.config().withLex(Lex.JAVA))
        .defaultSchema(rootSchema)
        .context(Contexts.EMPTY_CONTEXT)
        .costFactory(null)
        .typeSystem(RelDataTypeSystem.DEFAULT)
        .build();

    this.planner = Frameworks.getPlanner(calciteFrameworkConfig);
  }

  RelNode getLogicalPlan(String query)
      throws ValidationException, RelConversionException, SqlParseException, Exception {
    SqlNode sqlNode;

    sqlNode = planner.parse(query);
    InsertTableSchema(sqlNode);

    SqlNode validatedSqlNode = planner.validate(sqlNode);
    RelNode root = planner.rel(validatedSqlNode).project();

    planner.close();
    return root;
  }

  public void InsertTableSchema(SqlNode sqlNode) throws Exception {
    if (sqlNode instanceof SqlSelect) {
      SqlSelect select = (SqlSelect) sqlNode;

      SqlNode fromNode = select.getFrom();
      SqlIdentifier table = null;

      if (fromNode instanceof SqlIdentifier) {
        table = (SqlIdentifier) fromNode;
      } else if (fromNode instanceof SqlJoin) {
        SqlJoin join = (SqlJoin) fromNode;
        SqlNode leftTable = join.getLeft();
        SqlNode rightTable = join.getRight();

        if (leftTable instanceof SqlIdentifier) {
          SqlIdentifier leftTableName = (SqlIdentifier) leftTable;
          Map<String, List<String>> schemaMap = GetColumnsForSchema(leftTableName, join);
          addTableToSchema(leftTableName, select, true, schemaMap);
        }

        if (rightTable instanceof SqlIdentifier) {
          SqlIdentifier rightTableName = (SqlIdentifier) rightTable;
          addTableToSchema(rightTableName, select, false, null);
        }
      }

      if (table != null) {
        rootSchema.add(table.getSimple(), new AbstractTable() {
          @Override
          public RelDataType getRowType(RelDataTypeFactory typeFactory) {
            return buildRowType(select, typeFactory, false, null);
          }
        });
      }
    }
  }

  private void addTableToSchema(SqlIdentifier tableName, SqlSelect select, boolean isJoin,
      Map<String, List<String>> map) {

    if (!isJoin) {
      rootSchema.add(tableName.getSimple(), new AbstractTable() {
        @Override
        public RelDataType getRowType(RelDataTypeFactory typeFactory) {
          return buildRowType(select, typeFactory, false, null);
        }
      });
    } else {
      for (Map.Entry<String, List<String>> entry : map.entrySet()) {
        String tableNameJoin = entry.getKey();
        List<String> columnNames = entry.getValue();

        System.out.println("table: " + tableNameJoin);
        rootSchema.add(tableNameJoin, new AbstractTable() {
          @Override
          public RelDataType getRowType(RelDataTypeFactory typeFactory) {
            return buildRowType(select, typeFactory, isJoin, columnNames);
          }
        });
      }

    }
  }

  private RelDataType buildRowType(SqlSelect select, RelDataTypeFactory typeFactory, boolean isJoin,
      List<String> cols) {
    RelDataTypeFactory.Builder builder = typeFactory.builder();
    for (SqlNode selectItem : select.getSelectList()) {
      if (selectItem instanceof SqlIdentifier) {
        SqlIdentifier column = (SqlIdentifier) selectItem;
        builder.add(column.toString(),
            typeFactory.createTypeWithNullability(typeFactory.createSqlType(SqlTypeName.VARCHAR), true));
      }
    }

    if (isJoin) {
      for (String column : cols) {
        System.out.println(column);
        builder.add(column.toString(),
            typeFactory.createTypeWithNullability(typeFactory.createSqlType(SqlTypeName.VARCHAR), true));
      }

    }
    return builder.build();
  }

  private Map<String, List<String>> GetColumnsForSchema(SqlIdentifier tableName, SqlJoin join) throws Exception {
    Map<String, List<String>> map = new HashMap<>();
    SqlNode joinCondition = join.getCondition();

    if (joinCondition instanceof SqlBasicCall) {
      SqlBasicCall condition = (SqlBasicCall) joinCondition;

      for (SqlNode operand : condition.getOperandList()) {
        extractTableAndColumn(operand, map);
      }
    }

    return map;
  }

  private void extractTableAndColumn(SqlNode operand, Map<String, List<String>> map) {
    if (operand instanceof SqlBasicCall) {
      for (SqlNode subOperand : ((SqlBasicCall) operand).getOperandList()) {
        extractTableAndColumn(subOperand, map);
      }
    } else if (operand instanceof SqlIdentifier) {
      SqlIdentifier identifier = (SqlIdentifier) operand;

      String tableName = identifier.names.get(0);
      String columnName = identifier.names.get(1);

      map.computeIfAbsent(tableName, k -> new ArrayList<>()).add(columnName);
    } else {
      System.out.println("Unhandled operand type: " + operand.getClass().getName());
    }
  }

  public static void main(String[] args)
      throws IOException, SQLException, ValidationException, RelConversionException, SqlParseException, Exception {
    SimpleQueryPlanner queryPlanner = new SimpleQueryPlanner();
    RelNode loginalPlan = queryPlanner
        .getLogicalPlan("SELECT Employees.Name, Departments.DepartmentName\n" + //
            "FROM Employees\n" + //
            "JOIN Departments ON Employees.DepartmentID = Departments.DepartmentID AND Departments.DepartmentID = 1828128");
    System.out.println(RelOptUtil.toString(loginalPlan));

    RelNode loginalPlan2 = queryPlanner.getLogicalPlan("SELECT name, age FROM users");
    System.out.println(RelOptUtil.toString(loginalPlan2));

    RelNode loginalPlan3 = queryPlanner.getLogicalPlan("SELECT wife, kids FROM husband");
    System.out.println(RelOptUtil.toString(loginalPlan3));

    RelNode loginalPlan4 = queryPlanner.getLogicalPlan("SELECT dad, mom FROM kid");
    System.out.println(RelOptUtil.toString(loginalPlan4));
  }
}