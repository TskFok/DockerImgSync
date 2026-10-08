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
	called := false
	created, err := ensureTables("img_", func(table string) (bool, error) {
		return true, nil
	}, func(stmt string) error {
		called = true
		return nil
	})
	if err != nil {
		t.Fatalf("表已齐全时不应失败: %v", err)
	}
	if len(created) != 0 {
		t.Fatalf("表已齐全时不应建表，得到 %v", created)
	}
	if called {
		t.Fatal("表已齐全时不应执行建表语句")
	}
}

func TestEnsureTablesCreatesWhenAnyMissing(t *testing.T) {
	var executed []string
	created, err := ensureTables("img_", func(table string) (bool, error) {
		return table != "img_registry", nil
	}, func(stmt string) error {
		executed = append(executed, stmt)
		return nil
	})
	if err != nil {
		t.Fatalf("缺表时建表失败: %v", err)
	}
	if len(created) != 1 || created[0] != "img_registry" {
		t.Fatalf("缺失表 = %v，期望 [img_registry]", created)
	}
	if len(executed) != 4 {
		t.Fatalf("应执行全部建表语句，实际 %d 条", len(executed))
	}
	if !strings.Contains(executed[0], "`img_credential`") {
		t.Fatal("应按外键依赖顺序先创建 credential")
	}
}

func TestEnsureTablesReturnsCheckError(t *testing.T) {
	_, err := ensureTables("", func(table string) (bool, error) {
		return false, errors.New("db down")
	}, func(stmt string) error {
		t.Fatal("检查失败时不应建表")
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "credential") {
		t.Fatalf("错误 = %v，期望包含表名", err)
	}
}

func TestEnsureTablesReturnsExecError(t *testing.T) {
	_, err := ensureTables("", func(table string) (bool, error) {
		return false, nil
	}, func(stmt string) error {
		return errors.New("denied")
	})
	if err == nil || !strings.Contains(err.Error(), "建表失败") {
		t.Fatalf("错误 = %v，期望建表失败", err)
	}
}
