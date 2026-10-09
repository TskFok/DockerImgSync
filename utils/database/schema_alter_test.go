package database

import (
	"database/sql"
	"strings"
	"testing"
)

func mysqlDisplayColumnType(typeName, columnName string) string {
	switch typeName {
	case "tinyint":
		if columnName == "enabled" {
			return "tinyint(1)"
		}
		return "tinyint(4)"
	case "smallint":
		return "smallint(6)"
	case "mediumint":
		return "mediumint(9)"
	case "int":
		return "int(11)"
	case "bigint":
		return "bigint(20)"
	default:
		return typeName
	}
}

func liveRowsMySQLStyle(tables []tableSpec) (names []string, present []tableNameRow, cols []columnRow, indexes []indexRow, fks []fkRow) {
	for _, table := range tables {
		names = append(names, table.Name)
		present = append(present, tableNameRow{TableName: table.Name})
		for _, col := range table.Columns {
			row := columnRow{
				TableName:  table.Name,
				ColumnName: col.Name,
				ColumnType: mysqlDisplayColumnType(col.TypeName, col.Name),
				IsNullable: "NO",
			}
			if col.Nullable {
				row.IsNullable = "YES"
			}
			if col.AutoIncrement {
				row.Extra = "auto_increment"
			}
			if col.HasDefault {
				row.ColumnDefault = sql.NullString{String: col.DefaultValue, Valid: true}
			}
			cols = append(cols, row)
		}
		for seq, pkCol := range table.PrimaryKey {
			indexes = append(indexes, indexRow{
				TableName: table.Name, IndexName: "PRIMARY", NonUnique: 0,
				SeqInIndex: int64(seq + 1), ColumnName: pkCol,
			})
		}
		for _, idx := range table.Indexes {
			nonUnique := int64(1)
			if idx.Unique {
				nonUnique = 0
			}
			for seq, colName := range idx.Columns {
				indexes = append(indexes, indexRow{
					TableName: table.Name, IndexName: idx.Name, NonUnique: nonUnique,
					SeqInIndex: int64(seq + 1), ColumnName: colName,
				})
			}
		}
		for _, fk := range table.ForeignKeys {
			for ord, colName := range fk.Columns {
				fks = append(fks, fkRow{
					TableName: table.Name, ConstraintName: fk.Name, ColumnName: colName,
					Ordinal: int64(ord + 1), RefTable: fk.RefTable, RefColumn: fk.RefColumns[ord],
					DeleteRule: fk.OnDelete,
				})
			}
		}
	}
	return names, present, cols, indexes, fks
}

func TestDiffTableNoAlterWhenMySQLIntegerDisplayWidths(t *testing.T) {
	tables, err := desiredTables("")
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	names, present, cols, indexes, fks := liveRowsMySQLStyle(tables)
	_, live := assembleLive(names, present, cols, indexes, fks)
	for _, spec := range tables {
		alt := diffTable(spec, live[spec.Name])
		stmt, ok := renderAlter(spec.Name, alt)
		if ok || stmt != "" {
			t.Fatalf("表 %s 不应生成 ALTER: ok=%v stmt=%q diff=%+v", spec.Name, ok, stmt, alt)
		}
	}
}

func TestAlterAddsAndModifiesColumnsWithoutDroppingExtras(t *testing.T) {
	tables, err := desiredTables("")
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	reg := tables[1]
	live := matchingLive(reg)
	delete(live.Columns, "namespace")
	addr := live.Columns["address"]
	addr.TypeName = "varchar(128)"
	live.Columns["address"] = addr
	note := live.Columns["name"]
	note.Nullable = true
	live.Columns["name"] = note
	live.Columns["legacy"] = liveColumn{Name: "legacy", TypeName: "int", Nullable: true}

	stmt, ok := renderAlter(reg.Name, diffTable(reg, live))
	if !ok {
		t.Fatal("应生成 ALTER")
	}
	if !strings.Contains(stmt, "ADD COLUMN `namespace` varchar(255) NOT NULL") {
		t.Fatalf("缺少 ADD COLUMN: %s", stmt)
	}
	if !strings.Contains(stmt, "MODIFY COLUMN `address` varchar(255) NOT NULL") {
		t.Fatalf("缺少类型修改: %s", stmt)
	}
	if !strings.Contains(stmt, "MODIFY COLUMN `name` varchar(255) NOT NULL") {
		t.Fatalf("缺少可空修改: %s", stmt)
	}
	if strings.Contains(stmt, "DROP COLUMN") || strings.Contains(stmt, "legacy") {
		t.Fatalf("不应删除多余列: %s", stmt)
	}
	addAt := strings.Index(stmt, "ADD COLUMN")
	modAt := strings.Index(stmt, "MODIFY COLUMN")
	if addAt < 0 || modAt < 0 || addAt > modAt {
		t.Fatalf("ADD COLUMN 应在 MODIFY COLUMN 之前: %s", stmt)
	}
}

func TestAlterModifiesDefaultAndSkipsMatchingTable(t *testing.T) {
	tables, err := desiredTables("")
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	task := tables[2]
	live := matchingLive(task)
	interval := live.Columns["interval_seconds"]
	interval.HasDefault = false
	interval.DefaultValue = ""
	live.Columns["interval_seconds"] = interval

	stmt, ok := renderAlter(task.Name, diffTable(task, live))
	if !ok || !strings.Contains(stmt, "MODIFY COLUMN `interval_seconds` int NOT NULL DEFAULT 0") {
		t.Fatalf("默认值差异未生成 MODIFY: %s", stmt)
	}

	same, ok := renderAlter(task.Name, diffTable(task, matchingLive(task)))
	if ok || same != "" {
		t.Fatalf("一致的表不应生成 ALTER: %q", same)
	}
}

func matchingLive(spec tableSpec) liveTable {
	live := liveTable{
		Columns:     map[string]liveColumn{},
		PrimaryKey:  append([]string{}, spec.PrimaryKey...),
		Indexes:     map[string]liveIndex{},
		ForeignKeys: map[string]liveForeignKey{},
	}
	for _, col := range spec.Columns {
		live.Columns[col.Name] = liveColumn{
			Name:          col.Name,
			TypeName:      col.TypeName,
			Nullable:      col.Nullable,
			HasDefault:    col.HasDefault,
			DefaultValue:  col.DefaultValue,
			AutoIncrement: col.AutoIncrement,
		}
	}
	for _, idx := range spec.Indexes {
		live.Indexes[idx.Name] = liveIndex{Name: idx.Name, Unique: idx.Unique, Columns: append([]string{}, idx.Columns...)}
	}
	for _, fk := range spec.ForeignKeys {
		live.ForeignKeys[fk.Name] = liveForeignKey{
			Name:       fk.Name,
			Columns:    append([]string{}, fk.Columns...),
			RefTable:   fk.RefTable,
			RefColumns: append([]string{}, fk.RefColumns...),
			OnDelete:   fk.OnDelete,
		}
	}
	return live
}

func TestAlterAddsMissingIndexAndForeignKey(t *testing.T) {
	tables, err := desiredTables("img_")
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	reg := tables[1]
	live := matchingLive(reg)
	delete(live.Indexes, "uk_registry_name")
	delete(live.ForeignKeys, "fk_registry_credential")
	live.Indexes["idx_extra"] = liveIndex{Name: "idx_extra", Columns: []string{"address"}}
	live.ForeignKeys["fk_legacy"] = liveForeignKey{Name: "fk_legacy", Columns: []string{"address"}, RefTable: "credential", RefColumns: []string{"id"}, OnDelete: "RESTRICT"}

	stmt, ok := renderAlter(reg.Name, diffTable(reg, live))
	if !ok {
		t.Fatal("应生成 ALTER")
	}
	if !strings.Contains(stmt, "ADD UNIQUE KEY `uk_registry_name` (`name`)") {
		t.Fatalf("缺少索引: %s", stmt)
	}
	if !strings.Contains(stmt, "ADD CONSTRAINT `fk_registry_credential` FOREIGN KEY (`credential_id`) REFERENCES `img_credential` (`id`) ON DELETE RESTRICT") {
		t.Fatalf("缺少外键: %s", stmt)
	}
	if strings.Contains(stmt, "idx_extra") || strings.Contains(stmt, "fk_legacy") || strings.Contains(stmt, "DROP INDEX") || strings.Contains(stmt, "DROP FOREIGN KEY") {
		t.Fatalf("不应删除缺失重建之外的对象: %s", stmt)
	}
}

func TestAlterRecreatesIndexWhenColumnsDiffer(t *testing.T) {
	tables, err := desiredTables("")
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	reg := tables[1]
	live := matchingLive(reg)
	live.Indexes["uk_registry_name"] = liveIndex{Name: "uk_registry_name", Unique: true, Columns: []string{"address"}}

	stmt, ok := renderAlter(reg.Name, diffTable(reg, live))
	if !ok {
		t.Fatal("应生成 ALTER")
	}
	dropAt := strings.Index(stmt, "DROP INDEX `uk_registry_name`")
	addAt := strings.Index(stmt, "ADD UNIQUE KEY `uk_registry_name` (`name`)")
	if dropAt < 0 || addAt < 0 || dropAt > addAt {
		t.Fatalf("应先删同名索引再重建: %s", stmt)
	}
}

func TestPlanSeparatesSameNameForeignKeyRecreate(t *testing.T) {
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
	source := live["sync_task"].Columns["source_credential_id"]
	source.Nullable = false
	live["sync_task"].Columns["source_credential_id"] = source
	next := live["sync_task"].Columns["next_run_at"]
	next.Nullable = false
	live["sync_task"].Columns["next_run_at"] = next

	stmts, _, _ := planStatements(tables, exists, live)
	var taskSQL []string
	for _, stmt := range stmts {
		if stmt.Table == "sync_task" {
			taskSQL = append(taskSQL, stmt.SQL)
		}
	}
	if len(taskSQL) < 2 {
		t.Fatalf("同名外键的删除和重建应分成至少两条 ALTER: %#v", taskSQL)
	}
	dropAt, modAt, addAt := -1, -1, -1
	for i, sql := range taskSQL {
		hasDrop := strings.Contains(sql, "DROP FOREIGN KEY `fk_sync_task_source_credential`")
		hasAdd := strings.Contains(sql, "ADD CONSTRAINT `fk_sync_task_source_credential`")
		if hasDrop && hasAdd {
			t.Fatalf("同一条 ALTER 不能同时删除并重建同名外键: %s", sql)
		}
		if hasDrop {
			dropAt = i
		}
		if strings.Contains(sql, "MODIFY COLUMN `source_credential_id`") {
			modAt = i
		}
		if hasAdd {
			addAt = i
		}
	}
	if dropAt < 0 || modAt < 0 || addAt < 0 || dropAt != modAt || !(modAt < addAt) {
		t.Fatalf("应先在改列语句里删除外键，再单独加回: %#v", taskSQL)
	}
	if !strings.Contains(taskSQL[dropAt], "DROP INDEX `idx_sync_task_due`") || !strings.Contains(taskSQL[dropAt], "ADD KEY `idx_sync_task_due`") {
		t.Fatalf("索引重建应留在改列语句中: %s", taskSQL[dropAt])
	}
}

func TestAlterRecreatesForeignKeyWhenColumnChanges(t *testing.T) {
	tables, err := desiredTables("")
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	reg := tables[1]
	live := matchingLive(reg)
	col := live.Columns["credential_id"]
	col.TypeName = "bigint"
	live.Columns["credential_id"] = col

	stmt, ok := renderAlter(reg.Name, diffTable(reg, live))
	if !ok {
		t.Fatal("应生成 ALTER")
	}
	dropAt := strings.Index(stmt, "DROP FOREIGN KEY `fk_registry_credential`")
	modAt := strings.Index(stmt, "MODIFY COLUMN `credential_id` int NOT NULL")
	addAt := strings.Index(stmt, "ADD CONSTRAINT `fk_registry_credential`")
	if dropAt < 0 || modAt < 0 || addAt < 0 || !(dropAt < modAt && modAt < addAt) {
		t.Fatalf("外键、改列、重建外键的顺序不对: %s", stmt)
	}
}

func TestAlterRecreatesPrimaryKeyAndIndexOnModifiedColumn(t *testing.T) {
	tables, err := desiredTables("")
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	cred := tables[0]
	live := matchingLive(cred)
	id := live.Columns["id"]
	id.TypeName = "bigint"
	live.Columns["id"] = id
	name := live.Columns["name"]
	name.TypeName = "varchar(64)"
	live.Columns["name"] = name

	stmt, ok := renderAlter(cred.Name, diffTable(cred, live))
	if !ok {
		t.Fatal("应生成 ALTER")
	}
	dropPK := strings.Index(stmt, "DROP PRIMARY KEY")
	modID := strings.Index(stmt, "MODIFY COLUMN `id` int NOT NULL AUTO_INCREMENT")
	addPK := strings.Index(stmt, "ADD PRIMARY KEY (`id`)")
	dropIdx := strings.Index(stmt, "DROP INDEX `uk_credential_name`")
	modName := strings.Index(stmt, "MODIFY COLUMN `name` varchar(255) NOT NULL")
	addIdx := strings.Index(stmt, "ADD UNIQUE KEY `uk_credential_name` (`name`)")
	if dropPK < 0 || modID < 0 || addPK < 0 || dropIdx < 0 || modName < 0 || addIdx < 0 {
		t.Fatalf("主键或索引未随列修改重建: %s", stmt)
	}
	if !(dropPK < modID && modID < addPK && dropIdx < modName && modName < addIdx) {
		t.Fatalf("删除、修改、重建的顺序不对: %s", stmt)
	}
}

func TestPlanDropsReferencingForeignKeyBeforeColumnChange(t *testing.T) {
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
	id := live["img_credential"].Columns["id"]
	id.TypeName = "bigint"
	live["img_credential"].Columns["id"] = id

	stmts, created, altered := planStatements(tables, exists, live)
	if len(created) != 0 {
		t.Fatalf("不应建表: %v", created)
	}
	sqls := make([]string, len(stmts))
	for i, stmt := range stmts {
		sqls[i] = stmt.SQL
	}
	joined := strings.Join(sqls, "\n")
	regDrop := strings.Index(joined, "ALTER TABLE `img_registry` DROP FOREIGN KEY `fk_registry_credential`")
	taskDrop := strings.Index(joined, "ALTER TABLE `img_sync_task` DROP FOREIGN KEY `fk_sync_task_source_credential`")
	mod := strings.Index(joined, "ALTER TABLE `img_credential`")
	regAdd := strings.Index(joined, "ALTER TABLE `img_registry` ADD CONSTRAINT `fk_registry_credential`")
	taskAdd := strings.Index(joined, "ADD CONSTRAINT `fk_sync_task_source_credential`")
	if regDrop < 0 || taskDrop < 0 || mod < 0 || regAdd < 0 || taskAdd < 0 {
		t.Fatalf("语句不完整:\n%s", joined)
	}
	if !(regDrop < mod && taskDrop < mod && mod < regAdd && mod < taskAdd) {
		t.Fatalf("应先删引用外键，再改 credential.id，再加回外键:\n%s", joined)
	}
	if strings.Contains(sqls[0], "ADD CONSTRAINT") {
		t.Fatalf("第一条不应在改列前加回外键: %s", sqls[0])
	}
	seen := map[string]bool{}
	for _, name := range altered {
		if seen[name] {
			t.Fatalf("修改列表重复: %v", altered)
		}
		seen[name] = true
	}
	for _, name := range []string{"img_credential", "img_registry", "img_sync_task"} {
		if !seen[name] {
			t.Fatalf("修改列表 = %v，缺少 %s", altered, name)
		}
	}
}

func TestPlanCreatesReferencingTableAfterColumnChange(t *testing.T) {
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
	exists["registry"] = false
	id := live["credential"].Columns["id"]
	id.TypeName = "bigint"
	live["credential"].Columns["id"] = id

	stmts, created, _ := planStatements(tables, exists, live)
	modAt := -1
	createAt := -1
	for i, stmt := range stmts {
		if strings.Contains(stmt.SQL, "ALTER TABLE `credential`") && strings.Contains(stmt.SQL, "MODIFY COLUMN `id`") {
			modAt = i
		}
		if stmt.Create && stmt.Table == "registry" {
			createAt = i
		}
	}
	if modAt < 0 || createAt < 0 || createAt < modAt {
		t.Fatalf("registry 的 CREATE 应晚于 credential.id 的修改: %#v", stmts)
	}
	if len(created) != 1 || created[0] != "registry" {
		t.Fatalf("新建 = %v", created)
	}
}

func TestPlanKeepsSingleAlterWhenChangeIsLocal(t *testing.T) {
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
	delete(live["registry"].Columns, "namespace")

	stmts, created, altered := planStatements(tables, exists, live)
	if len(created) != 0 || len(altered) != 1 || altered[0] != "registry" || len(stmts) != 1 {
		t.Fatalf("语句 = %#v，新建 = %v，修改 = %v", stmts, created, altered)
	}
	if !strings.Contains(stmts[0].SQL, "ADD COLUMN `namespace` varchar(255) NOT NULL") {
		t.Fatalf("语句 = %s", stmts[0].SQL)
	}
}

func TestPlanDelaysMissingChildOfDelayedTable(t *testing.T) {
	tables, err := desiredTables("")
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	exists := map[string]bool{"credential": true}
	live := map[string]liveTable{"credential": matchingLive(tables[0])}
	id := live["credential"].Columns["id"]
	id.TypeName = "bigint"
	live["credential"].Columns["id"] = id

	stmts, created, _ := planStatements(tables, exists, live)
	modAt := -1
	createAt := map[string]int{}
	for i, stmt := range stmts {
		if strings.Contains(stmt.SQL, "ALTER TABLE `credential`") && strings.Contains(stmt.SQL, "MODIFY COLUMN `id`") {
			modAt = i
		}
		if stmt.Create {
			createAt[stmt.Table] = i
		}
	}
	if modAt < 0 {
		t.Fatalf("缺少 credential.id 的修改: %#v", stmts)
	}
	for _, name := range []string{"registry", "sync_task", "sync_log"} {
		at, ok := createAt[name]
		if !ok || at < modAt {
			t.Fatalf("%s 的 CREATE 应晚于 credential 的修改: %#v", name, stmts)
		}
	}
	if !(createAt["registry"] < createAt["sync_task"] && createAt["sync_task"] < createAt["sync_log"]) {
		t.Fatalf("CREATE 顺序应为 registry、sync_task、sync_log: %#v", stmts)
	}
	if strings.Join(created, ",") != "registry,sync_task,sync_log" {
		t.Fatalf("新建 = %v", created)
	}
}

func TestPlanAddsForeignKeyAfterDelayedReferencedTableCreate(t *testing.T) {
	tables, err := desiredTables("")
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	exists := map[string]bool{"credential": true, "sync_task": true}
	live := map[string]liveTable{
		"credential": matchingLive(tables[0]),
		"sync_task":  matchingLive(tables[2]),
	}
	id := live["credential"].Columns["id"]
	id.TypeName = "bigint"
	live["credential"].Columns["id"] = id
	delete(live["sync_task"].ForeignKeys, "fk_sync_task_registry")

	stmts, _, _ := planStatements(tables, exists, live)
	modAt, createAt, addAt := -1, -1, -1
	for i, stmt := range stmts {
		if strings.Contains(stmt.SQL, "ALTER TABLE `credential`") && strings.Contains(stmt.SQL, "MODIFY COLUMN `id`") {
			modAt = i
		}
		if stmt.Create && stmt.Table == "registry" {
			createAt = i
		}
		if strings.Contains(stmt.SQL, "ADD CONSTRAINT `fk_sync_task_registry`") {
			if addAt >= 0 {
				t.Fatalf("fk_sync_task_registry 重复添加: %#v", stmts)
			}
			addAt = i
		}
	}
	if modAt < 0 || createAt < modAt {
		t.Fatalf("CREATE registry 应晚于 credential 的修改: %#v", stmts)
	}
	if addAt < createAt {
		t.Fatalf("ADD CONSTRAINT fk_sync_task_registry 应晚于 CREATE registry: %#v", stmts)
	}
	for _, stmt := range stmts {
		if strings.Contains(stmt.SQL, "DROP FOREIGN KEY `fk_sync_task_registry`") {
			t.Fatalf("不应删除不存在的外键: %s", stmt.SQL)
		}
	}
}
