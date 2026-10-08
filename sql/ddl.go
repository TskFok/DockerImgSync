package schemadef

import _ "embed"

// SQL 是不含表前缀的建表脚本。
//
//go:embed schema.sql
var SQL string
