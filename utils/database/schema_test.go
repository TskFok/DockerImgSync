package database

import (
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
