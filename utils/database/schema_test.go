package database

import (
	"database/sql"
	"errors"
	"strings"
	"testing"
)

func TestSchemaStatementsAddsPrefixAndSkipsExisting(t *testing.T) {
	stmts, err := SchemaStatements("img_")
	if err != nil {
		t.Fatalf("生成建表语句失败: %v", err)
	}
	if len(stmts) != 4 {
		t.Fatalf("语句数量 = %d，期望 4", len(stmts))
	}

	joined := strings.Join(stmts, "\n")
	for _, name := range []string{"`img_credential`", "`img_registry`", "`img_sync_task`", "`img_sync_log`"} {
		if !strings.Contains(joined, name) {
			t.Errorf("建表语句缺少 %s", name)
		}
	}
	for _, bare := range []string{"`credential`", "`registry`", "`sync_task`", "`sync_log`"} {
		if strings.Contains(joined, bare) {
			t.Errorf("加前缀后不应再出现 %s", bare)
		}
	}
	if strings.Contains(strings.ReplaceAll(joined, "CREATE TABLE IF NOT EXISTS `", ""), "CREATE TABLE `") {
		t.Fatal("建表语句应使用 CREATE TABLE IF NOT EXISTS")
	}
	if !strings.Contains(joined, "REFERENCES `img_credential`") {
		t.Fatal("外键引用的表名也应加前缀")
	}
	if !strings.Contains(joined, "REFERENCES `img_sync_task`") {
		t.Fatal("sync_log 外键引用的表名也应加前缀")
	}
}

func TestSchemaStatementsKeepsNamesWhenPrefixEmpty(t *testing.T) {
	stmts, err := SchemaStatements("")
	if err != nil {
		t.Fatalf("生成建表语句失败: %v", err)
	}
	joined := strings.Join(stmts, "\n")
	if !strings.Contains(joined, "CREATE TABLE IF NOT EXISTS `credential`") {
		t.Fatal("空前缀时应保留原表名")
	}
	if strings.Contains(joined, "CREATE TABLE `credential`") {
		t.Fatal("空前缀时也应避免重复创建")
	}
}

func TestSchemaStatementsRejectsUnsafePrefix(t *testing.T) {
	_, err := SchemaStatements("img-")
	if err == nil {
		t.Fatal("含非法字符的前缀应返回错误")
	}
}

func TestEnsureTablesSkipsWhenComplete(t *testing.T) {
	tables, err := desiredTables("img_")
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	exists := map[string]bool{}
	live := map[string]liveTable{}
	for _, table := range tables {
		exists[table.Name] = true
		live[table.Name] = matchingLive(table)
	}
	called := false
	result, err := ensureTables("img_", schemaProbe{
		load: func(names []string) (map[string]bool, map[string]liveTable, error) {
			if len(names) != 4 || names[0] != "img_credential" {
				t.Fatalf("读取的表 = %v", names)
			}
			return exists, live, nil
		},
		exec: func(stmt string) error {
			called = true
			return nil
		},
	})
	if err != nil {
		t.Fatalf("表已一致时不应失败: %v", err)
	}
	if len(result.Created) != 0 || len(result.Altered) != 0 || called {
		t.Fatalf("结果 = %+v，执行了语句 = %v", result, called)
	}
}

func TestEnsureTablesCreatesOnlyMissingTable(t *testing.T) {
	tables, err := desiredTables("img_")
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	exists := map[string]bool{}
	live := map[string]liveTable{}
	for _, table := range tables {
		exists[table.Name] = table.Name != "img_registry"
		if exists[table.Name] {
			live[table.Name] = matchingLive(table)
		}
	}
	var executed []string
	result, err := ensureTables("img_", schemaProbe{
		load: func(names []string) (map[string]bool, map[string]liveTable, error) {
			return exists, live, nil
		},
		exec: func(stmt string) error {
			executed = append(executed, stmt)
			return nil
		},
	})
	if err != nil {
		t.Fatalf("缺表时失败: %v", err)
	}
	if len(result.Created) != 1 || result.Created[0] != "img_registry" || len(result.Altered) != 0 {
		t.Fatalf("结果 = %+v", result)
	}
	if len(executed) != 1 || !strings.Contains(executed[0], "CREATE TABLE IF NOT EXISTS `img_registry`") {
		t.Fatalf("应只创建 registry: %v", executed)
	}
	if !strings.Contains(executed[0], "REFERENCES `img_credential`") {
		t.Fatalf("建表语句应带前缀外键: %s", executed[0])
	}
}

func TestEnsureTablesReturnsReadErrorWithoutExec(t *testing.T) {
	called := false
	_, err := ensureTables("", schemaProbe{
		load: func(names []string) (map[string]bool, map[string]liveTable, error) {
			return nil, nil, errors.New("检查数据表失败: db down")
		},
		exec: func(stmt string) error {
			called = true
			return nil
		},
	})
	if err == nil || !strings.Contains(err.Error(), "检查数据表失败") || called {
		t.Fatalf("错误 = %v，已执行 = %v", err, called)
	}
}

func TestEnsureTablesStopsAfterCreateError(t *testing.T) {
	var executed []string
	_, err := ensureTables("", schemaProbe{
		load: func(names []string) (map[string]bool, map[string]liveTable, error) {
			exists := map[string]bool{}
			for _, name := range names {
				exists[name] = false
			}
			return exists, map[string]liveTable{}, nil
		},
		exec: func(stmt string) error {
			executed = append(executed, stmt)
			return errors.New("denied")
		},
	})
	if err == nil || !strings.Contains(err.Error(), "建表失败:") || !strings.Contains(err.Error(), "credential") {
		t.Fatalf("错误 = %v", err)
	}
	if len(executed) != 1 {
		t.Fatalf("失败后不应继续执行，语句数 = %d", len(executed))
	}
}

func TestEnsureTablesStopsAfterAlterError(t *testing.T) {
	tables, err := desiredTables("")
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	exists := map[string]bool{}
	live := map[string]liveTable{}
	for _, table := range tables {
		exists[table.Name] = true
		live[table.Name] = matchingLive(table)
	}
	delete(live["credential"].Columns, "name")
	delete(live["registry"].Columns, "namespace")
	executed := 0
	_, err = ensureTables("", schemaProbe{
		load: func(names []string) (map[string]bool, map[string]liveTable, error) {
			return exists, live, nil
		},
		exec: func(stmt string) error {
			executed++
			return errors.New("denied")
		},
	})
	if err == nil || !strings.Contains(err.Error(), "修改表结构失败:") || !strings.Contains(err.Error(), "credential") {
		t.Fatalf("错误 = %v", err)
	}
	if executed != 1 {
		t.Fatalf("失败后不应继续执行，语句数 = %d", executed)
	}
}

func TestEnsureTablesRejectsPrefixBeforeLoad(t *testing.T) {
	called := false
	_, err := ensureTables("img-", schemaProbe{
		load: func(names []string) (map[string]bool, map[string]liveTable, error) {
			called = true
			return nil, nil, nil
		},
		exec: func(stmt string) error {
			called = true
			return nil
		},
	})
	if err == nil || !strings.Contains(err.Error(), "MYSQL_PREFIX 只能包含字母、数字和下划线") || called {
		t.Fatalf("错误 = %v，已查库 = %v", err, called)
	}
}

func TestColumnFromRowNormalizesMetadata(t *testing.T) {
	absent := columnFromRow(columnRow{
		ColumnName: "name",
		ColumnType: "varchar(255)",
		IsNullable: "NO",
		Extra:      "",
	})
	if absent.HasDefault || absent.Nullable || absent.TypeName != "varchar(255)" || absent.AutoIncrement {
		t.Fatalf("无默认值列 = %+v", absent)
	}
	empty := columnFromRow(columnRow{
		ColumnName:    "dest_repository",
		ColumnType:    "VARCHAR(255)",
		IsNullable:    "NO",
		ColumnDefault: sql.NullString{String: "", Valid: true},
	})
	if !empty.HasDefault || empty.DefaultValue != "" || empty.TypeName != "varchar(255)" {
		t.Fatalf("空字符串默认值 = %+v", empty)
	}
	id := columnFromRow(columnRow{
		ColumnName: "id",
		ColumnType: "int",
		IsNullable: "NO",
		Extra:      "auto_increment",
	})
	if !id.AutoIncrement {
		t.Fatal("应识别 AUTO_INCREMENT")
	}
}

func TestAssembleLiveGroupsColumnsIndexesAndForeignKeys(t *testing.T) {
	exists, live := assembleLive(
		[]string{"credential", "registry"},
		[]tableNameRow{{TableName: "credential"}},
		[]columnRow{
			{TableName: "credential", ColumnName: "id", ColumnType: "int", IsNullable: "NO", Extra: "auto_increment"},
			{TableName: "credential", ColumnName: "name", ColumnType: "varchar(255)", IsNullable: "NO"},
		},
		[]indexRow{
			{TableName: "credential", IndexName: "PRIMARY", NonUnique: 0, SeqInIndex: 1, ColumnName: "id"},
			{TableName: "credential", IndexName: "uk_credential_name", NonUnique: 0, SeqInIndex: 1, ColumnName: "name"},
		},
		[]fkRow{
			{TableName: "registry", ConstraintName: "fk_registry_credential", ColumnName: "credential_id", Ordinal: 1, RefTable: "credential", RefColumn: "id", DeleteRule: "RESTRICT"},
		},
	)
	if !exists["credential"] || exists["registry"] {
		t.Fatalf("存在性 = %v", exists)
	}
	cred := live["credential"]
	if !cred.Columns["id"].AutoIncrement || len(cred.PrimaryKey) != 1 || cred.PrimaryKey[0] != "id" {
		t.Fatalf("credential = %+v", cred)
	}
	if _, ok := cred.Indexes["PRIMARY"]; ok {
		t.Fatal("主键不应进入普通索引")
	}
	if !cred.Indexes["uk_credential_name"].Unique || cred.Indexes["uk_credential_name"].Columns[0] != "name" {
		t.Fatalf("索引 = %+v", cred.Indexes["uk_credential_name"])
	}
	fk := live["registry"].ForeignKeys["fk_registry_credential"]
	if fk.RefTable != "credential" || fk.OnDelete != "RESTRICT" || fk.Columns[0] != "credential_id" {
		t.Fatalf("外键 = %+v", fk)
	}
}

func TestLiveSchemaQueriesAreBatched(t *testing.T) {
	if len(liveSchemaQueries) != 4 {
		t.Fatalf("查询数量 = %d，期望 4", len(liveSchemaQueries))
	}
	joined := strings.ToLower(strings.Join(liveSchemaQueries, "\n"))
	for _, frag := range []string{
		"information_schema.tables",
		"information_schema.columns",
		"information_schema.statistics",
		"referential_constraints",
		"table_name in ?",
	} {
		if !strings.Contains(joined, frag) {
			t.Errorf("批量查询缺少 %s", frag)
		}
	}
}
