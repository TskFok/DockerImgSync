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
	if have.Indexes == nil {
		have.Indexes = map[string]liveIndex{}
	}
	if have.ForeignKeys == nil {
		have.ForeignKeys = map[string]liveForeignKey{}
	}
	var alt tableAlter
	modified := map[string]bool{}
	for _, col := range want.Columns {
		got, ok := have.Columns[col.Name]
		if !ok {
			alt.AddCols = append(alt.AddCols, col.Clause)
			continue
		}
		if !columnEqual(col, got) {
			modified[col.Name] = true
			alt.ModCols = append(alt.ModCols, col.Clause)
		}
	}
	for _, idx := range want.Indexes {
		got, ok := have.Indexes[idx.Name]
		if !ok || !indexEqual(idx, got) || overlaps(idx.Columns, modified) {
			if ok {
				alt.DropIdx = append(alt.DropIdx, idx.Name)
			}
			kind := "ADD KEY"
			if idx.Unique {
				kind = "ADD UNIQUE KEY"
			}
			alt.AddIdx = append(alt.AddIdx, kind+" `"+idx.Name+"` ("+quoteCols(idx.Columns)+")")
		}
	}
	if !equalStrings(want.PrimaryKey, have.PrimaryKey) || overlaps(want.PrimaryKey, modified) {
		if len(have.PrimaryKey) > 0 {
			alt.DropPK = true
		}
		alt.AddPK = true
		alt.PrimaryKey = append([]string{}, want.PrimaryKey...)
	}
	for _, fk := range want.ForeignKeys {
		got, ok := have.ForeignKeys[fk.Name]
		if !ok || !foreignKeyEqual(fk, got) || overlaps(fk.Columns, modified) {
			if ok {
				alt.DropFKs = append(alt.DropFKs, fk.Name)
			}
			alt.AddFKs = append(alt.AddFKs, fk.Clause)
		}
	}
	return alt
}

func indexEqual(want indexSpec, have liveIndex) bool {
	return want.Unique == have.Unique && equalStrings(want.Columns, have.Columns)
}

func foreignKeyEqual(want foreignKeySpec, have liveForeignKey) bool {
	return want.RefTable == have.RefTable && want.OnDelete == have.OnDelete &&
		equalStrings(want.Columns, have.Columns) && equalStrings(want.RefColumns, have.RefColumns)
}

func overlaps(cols []string, set map[string]bool) bool {
	for _, col := range cols {
		if set[col] {
			return true
		}
	}
	return false
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
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
