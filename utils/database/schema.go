package database

import (
	"database/sql"
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
			return loadLive(db, tables)
		},
		exec: func(stmt string) error {
			return db.Exec(stmt).Error
		},
	})
}

// information_schema 的结果列名在 MySQL 中是大写，gorm 按列标签区分大小写匹配，
// 所以每个查询列都显式起小写别名，和下面 row 结构体的 gorm 列标签一致。
const (
	sqlExistingTables = `
SELECT table_name AS table_name
FROM information_schema.tables
WHERE table_schema = DATABASE() AND table_name IN ?`

	sqlColumns = `
SELECT table_name AS table_name, column_name AS column_name, column_type AS column_type,
       is_nullable AS is_nullable, column_default AS column_default, extra AS extra
FROM information_schema.COLUMNS
WHERE table_schema = DATABASE() AND table_name IN ?`

	sqlIndexes = `
SELECT table_name AS table_name, index_name AS index_name, non_unique AS non_unique,
       seq_in_index AS seq_in_index, column_name AS column_name
FROM information_schema.STATISTICS
WHERE table_schema = DATABASE() AND table_name IN ?
ORDER BY table_name, index_name, seq_in_index`

	sqlForeignKeys = `
SELECT k.table_name AS table_name, k.constraint_name AS constraint_name, k.column_name AS column_name,
       k.ordinal_position AS ordinal_position, k.referenced_table_name AS referenced_table_name,
       k.referenced_column_name AS referenced_column_name, r.delete_rule AS delete_rule
FROM information_schema.KEY_COLUMN_USAGE k
JOIN information_schema.REFERENTIAL_CONSTRAINTS r
  ON r.constraint_schema = k.constraint_schema
 AND r.table_name = k.table_name
 AND r.constraint_name = k.constraint_name
WHERE k.table_schema = DATABASE()
  AND k.referenced_table_name IS NOT NULL
  AND k.table_name IN ?
ORDER BY k.table_name, k.constraint_name, k.ordinal_position`
)

var liveSchemaQueries = []string{sqlExistingTables, sqlColumns, sqlIndexes, sqlForeignKeys}

type tableNameRow struct {
	TableName string `gorm:"column:table_name"`
}

type columnRow struct {
	TableName     string         `gorm:"column:table_name"`
	ColumnName    string         `gorm:"column:column_name"`
	ColumnType    string         `gorm:"column:column_type"`
	IsNullable    string         `gorm:"column:is_nullable"`
	ColumnDefault sql.NullString `gorm:"column:column_default"`
	Extra         string         `gorm:"column:extra"`
}

type indexRow struct {
	TableName  string `gorm:"column:table_name"`
	IndexName  string `gorm:"column:index_name"`
	NonUnique  int64  `gorm:"column:non_unique"`
	SeqInIndex int64  `gorm:"column:seq_in_index"`
	ColumnName string `gorm:"column:column_name"`
}

type fkRow struct {
	TableName      string `gorm:"column:table_name"`
	ConstraintName string `gorm:"column:constraint_name"`
	ColumnName     string `gorm:"column:column_name"`
	Ordinal        int64  `gorm:"column:ordinal_position"`
	RefTable       string `gorm:"column:referenced_table_name"`
	RefColumn      string `gorm:"column:referenced_column_name"`
	DeleteRule     string `gorm:"column:delete_rule"`
}

// loadLive 按 liveSchemaQueries 的顺序固定执行四条查询，不随表数量增加。
func loadLive(db *gorm.DB, tables []string) (map[string]bool, map[string]liveTable, error) {
	var present []tableNameRow
	var cols []columnRow
	var indexes []indexRow
	var fks []fkRow
	dests := []interface{}{&present, &cols, &indexes, &fks}
	for i, query := range liveSchemaQueries {
		if err := db.Raw(query, tables).Scan(dests[i]).Error; err != nil {
			if i == 0 {
				return nil, nil, fmt.Errorf("检查数据表失败: %w", err)
			}
			return nil, nil, fmt.Errorf("读取表结构失败: %w", err)
		}
	}
	exists, live := assembleLive(tables, present, cols, indexes, fks)
	return exists, live, nil
}

func columnFromRow(row columnRow) liveColumn {
	return liveColumn{
		Name:          row.ColumnName,
		TypeName:      strings.ToLower(row.ColumnType),
		Nullable:      strings.EqualFold(row.IsNullable, "YES"),
		HasDefault:    row.ColumnDefault.Valid,
		DefaultValue:  row.ColumnDefault.String,
		AutoIncrement: strings.Contains(strings.ToLower(row.Extra), "auto_increment"),
	}
}

func assembleLive(want []string, present []tableNameRow, cols []columnRow, indexes []indexRow, fks []fkRow) (map[string]bool, map[string]liveTable) {
	exists := map[string]bool{}
	live := map[string]liveTable{}
	for _, name := range want {
		exists[name] = false
		live[name] = newLiveTable()
	}
	for _, row := range present {
		if _, ok := exists[row.TableName]; ok {
			exists[row.TableName] = true
		}
	}
	for _, row := range cols {
		table, ok := live[row.TableName]
		if !ok {
			continue
		}
		table.Columns[row.ColumnName] = columnFromRow(row)
		live[row.TableName] = table
	}
	for _, row := range indexes {
		table, ok := live[row.TableName]
		if !ok {
			continue
		}
		if strings.EqualFold(row.IndexName, "PRIMARY") {
			table.PrimaryKey = append(table.PrimaryKey, row.ColumnName)
			live[row.TableName] = table
			continue
		}
		idx := table.Indexes[row.IndexName]
		idx.Name = row.IndexName
		idx.Unique = row.NonUnique == 0
		idx.Columns = append(idx.Columns, row.ColumnName)
		table.Indexes[row.IndexName] = idx
		live[row.TableName] = table
	}
	for _, row := range fks {
		table, ok := live[row.TableName]
		if !ok {
			continue
		}
		fk := table.ForeignKeys[row.ConstraintName]
		fk.Name = row.ConstraintName
		fk.Columns = append(fk.Columns, row.ColumnName)
		fk.RefTable = row.RefTable
		fk.RefColumns = append(fk.RefColumns, row.RefColumn)
		fk.OnDelete = strings.ToUpper(row.DeleteRule)
		table.ForeignKeys[row.ConstraintName] = fk
		live[row.TableName] = table
	}
	return exists, live
}

func newLiveTable() liveTable {
	return liveTable{
		Columns:     map[string]liveColumn{},
		Indexes:     map[string]liveIndex{},
		ForeignKeys: map[string]liveForeignKey{},
	}
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
