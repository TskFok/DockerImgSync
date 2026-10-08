package database

import (
	"fmt"
	"strings"
)

type columnSpec struct {
	Name          string
	TypeName      string
	Nullable      bool
	HasDefault    bool
	DefaultValue  string
	AutoIncrement bool
	Clause        string
}

type indexSpec struct {
	Name    string
	Unique  bool
	Columns []string
}

type foreignKeySpec struct {
	Name       string
	Columns    []string
	RefTable   string
	RefColumns []string
	OnDelete   string
	Clause     string
}

type tableSpec struct {
	Name        string
	CreateSQL   string
	Columns     []columnSpec
	PrimaryKey  []string
	Indexes     []indexSpec
	ForeignKeys []foreignKeySpec
}

func desiredTables(prefix string) ([]tableSpec, error) {
	stmts, err := SchemaStatements(prefix)
	if err != nil {
		return nil, err
	}
	tables := make([]tableSpec, 0, len(stmts))
	for _, stmt := range stmts {
		table, err := parseCreate(stmt)
		if err != nil {
			return nil, fmt.Errorf("解析建表语句失败: %w", err)
		}
		tables = append(tables, table)
	}
	return tables, nil
}

func parseCreate(sql string) (tableSpec, error) {
	names := quotedNames(sql)
	if len(names) == 0 {
		return tableSpec{}, fmt.Errorf("建表语句缺少表名")
	}
	open := strings.IndexByte(sql, '(')
	if open < 0 {
		return tableSpec{}, fmt.Errorf("建表语句缺少列定义")
	}
	body, err := matchingParen(sql, open)
	if err != nil {
		return tableSpec{}, err
	}
	table := tableSpec{Name: names[0], CreateSQL: strings.TrimSpace(sql)}
	for _, part := range splitTopLevel(body) {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		upper := strings.ToUpper(part)
		switch {
		case strings.HasPrefix(upper, "PRIMARY KEY"):
			quoted := quotedNames(part)
			if len(quoted) == 0 {
				return tableSpec{}, fmt.Errorf("主键缺少列: %s", part)
			}
			table.PrimaryKey = quoted
		case strings.HasPrefix(upper, "UNIQUE KEY"):
			idx, err := parseIndex(part, true)
			if err != nil {
				return tableSpec{}, err
			}
			table.Indexes = append(table.Indexes, idx)
		case strings.HasPrefix(upper, "KEY "):
			idx, err := parseIndex(part, false)
			if err != nil {
				return tableSpec{}, err
			}
			table.Indexes = append(table.Indexes, idx)
		case strings.HasPrefix(upper, "CONSTRAINT"):
			fk, err := parseForeignKey(part)
			if err != nil {
				return tableSpec{}, err
			}
			table.ForeignKeys = append(table.ForeignKeys, fk)
		default:
			col, err := parseColumn(part)
			if err != nil {
				return tableSpec{}, err
			}
			table.Columns = append(table.Columns, col)
		}
	}
	if len(table.Columns) == 0 {
		return tableSpec{}, fmt.Errorf("表 %s 没有列", table.Name)
	}
	return table, nil
}

func parseColumn(clause string) (columnSpec, error) {
	names := quotedNames(clause)
	if len(names) == 0 {
		return columnSpec{}, fmt.Errorf("列定义缺少列名: %s", clause)
	}
	rest := strings.TrimSpace(clause[strings.Index(clause, "`"+names[0]+"`")+len(names[0])+2:])
	upper := strings.ToUpper(rest)
	col := columnSpec{
		Name:          names[0],
		Nullable:      true,
		AutoIncrement: strings.Contains(upper, "AUTO_INCREMENT"),
		Clause:        strings.TrimSpace(clause),
	}
	if strings.Contains(upper, "NOT NULL") {
		col.Nullable = false
	}
	if i := strings.Index(upper, "DEFAULT"); i >= 0 {
		col.HasDefault = true
		raw := strings.TrimSpace(rest[i+len("DEFAULT"):])
		if j := strings.Index(strings.ToUpper(raw), "AUTO_INCREMENT"); j >= 0 {
			raw = strings.TrimSpace(raw[:j])
		}
		col.DefaultValue = normalizeDefault(raw)
	}
	cut := len(rest)
	for _, marker := range []string{"NOT NULL", "NULL", "DEFAULT", "AUTO_INCREMENT"} {
		if i := strings.Index(upper, marker); i >= 0 && i < cut {
			cut = i
		}
	}
	col.TypeName = strings.ToLower(strings.TrimSpace(rest[:cut]))
	if col.TypeName == "" {
		return columnSpec{}, fmt.Errorf("列 %s 缺少类型", col.Name)
	}
	return col, nil
}

func normalizeDefault(raw string) string {
	raw = strings.TrimSpace(raw)
	if len(raw) >= 2 && raw[0] == '\'' && raw[len(raw)-1] == '\'' {
		return raw[1 : len(raw)-1]
	}
	return raw
}

func parseIndex(clause string, unique bool) (indexSpec, error) {
	names := quotedNames(clause)
	if len(names) < 2 {
		return indexSpec{}, fmt.Errorf("索引定义不完整: %s", clause)
	}
	return indexSpec{Name: names[0], Unique: unique, Columns: names[1:]}, nil
}

func parseForeignKey(clause string) (foreignKeySpec, error) {
	upper := strings.ToUpper(clause)
	fkAt := strings.Index(upper, "FOREIGN KEY")
	refAt := strings.Index(upper, "REFERENCES")
	delAt := strings.Index(upper, "ON DELETE")
	if fkAt < 0 || refAt < 0 || delAt < 0 {
		return foreignKeySpec{}, fmt.Errorf("无法解析外键: %s", clause)
	}
	local := quotedNames(clause[fkAt:refAt])
	refNames := quotedNames(clause[refAt:delAt])
	if len(local) == 0 || len(refNames) < 2 {
		return foreignKeySpec{}, fmt.Errorf("外键列不完整: %s", clause)
	}
	onDelete := strings.ToUpper(strings.Fields(strings.TrimSpace(clause[delAt+len("ON DELETE"):]))[0])
	constraint := quotedNames(clause[:fkAt])
	if len(constraint) == 0 {
		return foreignKeySpec{}, fmt.Errorf("外键缺少约束名: %s", clause)
	}
	return foreignKeySpec{
		Name:       constraint[0],
		Columns:    local,
		RefTable:   refNames[0],
		RefColumns: refNames[1:],
		OnDelete:   onDelete,
		Clause:     strings.TrimSpace(clause),
	}, nil
}

func quotedNames(s string) []string {
	var names []string
	for {
		start := strings.IndexByte(s, '`')
		if start < 0 {
			return names
		}
		rest := s[start+1:]
		end := strings.IndexByte(rest, '`')
		if end < 0 {
			return names
		}
		names = append(names, rest[:end])
		s = rest[end+1:]
	}
}

func matchingParen(s string, open int) (string, error) {
	depth := 0
	for i := open; i < len(s); i++ {
		switch s[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return s[open+1 : i], nil
			}
		}
	}
	return "", fmt.Errorf("建表语句括号不配对")
}

func splitTopLevel(body string) []string {
	var parts []string
	var b strings.Builder
	depth := 0
	inQuote := false
	for _, r := range body {
		switch {
		case r == '\'':
			inQuote = !inQuote
			b.WriteRune(r)
		case inQuote:
			b.WriteRune(r)
		case r == '(':
			depth++
			b.WriteRune(r)
		case r == ')':
			depth--
			b.WriteRune(r)
		case r == ',' && depth == 0:
			parts = append(parts, b.String())
			b.Reset()
		default:
			b.WriteRune(r)
		}
	}
	if strings.TrimSpace(b.String()) != "" {
		parts = append(parts, b.String())
	}
	return parts
}
