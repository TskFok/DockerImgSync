package database

import (
	"fmt"
	"strings"
	"unicode"

	schemadef "github.com/TskFok/DockerImgSync/sql"
	"gorm.io/gorm"
)

// tableNames 与 sql/schema.sql 中的建表顺序一致，先被引用的表在前。
var tableNames = []string{"credential", "registry", "sync_task", "sync_log"}

// SchemaResult 是本次启动新建和修改的表名，均含前缀。
type SchemaResult struct {
	Created []string
	Altered []string
}

type schemaProbe struct {
	load func(tables []string) (map[string]bool, map[string]liveTable, error)
	exec func(stmt string) error
}

// EnsureTables 按 sql/schema.sql 创建缺失的表，并修改已有表中与脚本不一致的列、索引和外键。
// 脚本里没有的列、索引和外键保留。
func EnsureTables(db *gorm.DB, prefix string) (SchemaResult, error) {
	return ensureTables(prefix, schemaProbe{
		load: func(tables []string) (map[string]bool, map[string]liveTable, error) {
			return nil, nil, fmt.Errorf("读取表结构失败: 尚未查询 information_schema")
		},
		exec: func(stmt string) error {
			return db.Exec(stmt).Error
		},
	})
}

func ensureTables(prefix string, probe schemaProbe) (SchemaResult, error) {
	if err := validatePrefix(prefix); err != nil {
		return SchemaResult{}, err
	}
	tables, err := desiredTables(prefix)
	if err != nil {
		return SchemaResult{}, err
	}
	names := make([]string, len(tables))
	for i, table := range tables {
		names[i] = table.Name
	}
	exists, live, err := probe.load(names)
	if err != nil {
		return SchemaResult{}, err
	}
	stmts, created, altered := planStatements(tables, exists, live)
	// 表数量固定为 4。跨表 DDL 不能并成一条语句，所以按已排好的顺序逐条执行。
	for _, stmt := range stmts {
		if err := probe.exec(stmt.SQL); err != nil {
			if stmt.Create {
				return SchemaResult{}, fmt.Errorf("建表失败: %s: %w", stmt.Table, err)
			}
			return SchemaResult{}, fmt.Errorf("修改表结构失败: %s: %w", stmt.Table, err)
		}
	}
	return SchemaResult{Created: created, Altered: altered}, nil
}

// SchemaStatements 把建表脚本转成可重复执行的语句，并给表名加上前缀。
func SchemaStatements(prefix string) ([]string, error) {
	if err := validatePrefix(prefix); err != nil {
		return nil, err
	}

	ddl := strings.ReplaceAll(schemadef.SQL, "CREATE TABLE `", "CREATE TABLE IF NOT EXISTS `")
	if prefix != "" {
		for _, name := range tableNames {
			ddl = strings.ReplaceAll(ddl, "`"+name+"`", "`"+prefix+name+"`")
		}
	}

	var stmts []string
	for _, part := range strings.Split(ddl, ";") {
		stmt := strings.TrimSpace(part)
		if stmt == "" || !strings.Contains(strings.ToUpper(stmt), "CREATE TABLE") {
			continue
		}
		stmts = append(stmts, stmt)
	}
	if len(stmts) != len(tableNames) {
		return nil, fmt.Errorf("schema.sql 应包含 %d 条建表语句，实际 %d 条", len(tableNames), len(stmts))
	}
	return stmts, nil
}

func validatePrefix(prefix string) error {
	for _, r := range prefix {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
			continue
		}
		return fmt.Errorf("MYSQL_PREFIX 只能包含字母、数字和下划线")
	}
	return nil
}
