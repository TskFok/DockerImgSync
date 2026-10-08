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
