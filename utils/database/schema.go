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

// EnsureTables 在缺少表时按 sql/schema.sql 创建，已有的表不会修改。
// 返回本次缺失并已创建的表名（含前缀）。
func EnsureTables(db *gorm.DB, prefix string) ([]string, error) {
	return ensureTables(prefix, func(table string) (bool, error) {
		var count int64
		err := db.Raw(
			"SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = DATABASE() AND table_name = ?",
			table,
		).Scan(&count).Error
		if err != nil {
			return false, err
		}
		return count > 0, nil
	}, func(stmt string) error {
		return db.Exec(stmt).Error
	})
}

func ensureTables(prefix string, exists func(string) (bool, error), exec func(string) error) ([]string, error) {
	var missing []string
	for _, name := range tableNames {
		table := prefix + name
		ok, err := exists(table)
		if err != nil {
			return nil, fmt.Errorf("检查数据表 %s 失败: %w", table, err)
		}
		if !ok {
			missing = append(missing, table)
		}
	}
	if len(missing) == 0 {
		return nil, nil
	}

	stmts, err := SchemaStatements(prefix)
	if err != nil {
		return nil, err
	}
	for _, stmt := range stmts {
		if err := exec(stmt); err != nil {
			return nil, fmt.Errorf("建表失败: %w", err)
		}
	}
	return missing, nil
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
