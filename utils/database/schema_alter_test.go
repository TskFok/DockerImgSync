package database

import (
	"strings"
	"testing"
)

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
