package database

import "strings"

type liveColumn struct {
	Name          string
	TypeName      string
	Nullable      bool
	HasDefault    bool
	DefaultValue  string
	AutoIncrement bool
}

type liveIndex struct {
	Name    string
	Unique  bool
	Columns []string
}

type liveForeignKey struct {
	Name       string
	Columns    []string
	RefTable   string
	RefColumns []string
	OnDelete   string
}

type liveTable struct {
	Columns     map[string]liveColumn
	PrimaryKey  []string
	Indexes     map[string]liveIndex
	ForeignKeys map[string]liveForeignKey
}

type tableAlter struct {
	DropFKs    []string
	DropIdx    []string
	DropPK     bool
	AddCols    []string
	ModCols    []string
	AddPK      bool
	PrimaryKey []string
	AddIdx     []string
	AddFKs     []string
}

func diffTable(want tableSpec, have liveTable) tableAlter {
	if have.Columns == nil {
		have.Columns = map[string]liveColumn{}
	}
	var alt tableAlter
	for _, col := range want.Columns {
		got, ok := have.Columns[col.Name]
		if !ok {
			alt.AddCols = append(alt.AddCols, col.Clause)
			continue
		}
		if !columnEqual(col, got) {
			alt.ModCols = append(alt.ModCols, col.Clause)
		}
	}
	return alt
}

func columnEqual(want columnSpec, have liveColumn) bool {
	if want.TypeName != have.TypeName || want.Nullable != have.Nullable || want.AutoIncrement != have.AutoIncrement {
		return false
	}
	if want.HasDefault != have.HasDefault {
		return false
	}
	if !want.HasDefault {
		return true
	}
	return want.DefaultValue == have.DefaultValue
}

func renderAlter(table string, alt tableAlter) (string, bool) {
	var parts []string
	for _, name := range alt.DropFKs {
		parts = append(parts, "DROP FOREIGN KEY `"+name+"`")
	}
	for _, name := range alt.DropIdx {
		parts = append(parts, "DROP INDEX `"+name+"`")
	}
	if alt.DropPK {
		parts = append(parts, "DROP PRIMARY KEY")
	}
	for _, clause := range alt.AddCols {
		parts = append(parts, "ADD COLUMN "+clause)
	}
	for _, clause := range alt.ModCols {
		parts = append(parts, "MODIFY COLUMN "+clause)
	}
	if alt.AddPK {
		parts = append(parts, "ADD PRIMARY KEY ("+quoteCols(alt.PrimaryKey)+")")
	}
	parts = append(parts, alt.AddIdx...)
	for _, clause := range alt.AddFKs {
		parts = append(parts, "ADD "+clause)
	}
	if len(parts) == 0 {
		return "", false
	}
	return "ALTER TABLE `" + table + "` " + strings.Join(parts, ", "), true
}

func quoteCols(cols []string) string {
	parts := make([]string, len(cols))
	for i, col := range cols {
		parts[i] = "`" + col + "`"
	}
	return strings.Join(parts, ", ")
}
