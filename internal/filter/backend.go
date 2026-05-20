package filter

import (
	"fmt"
	"strings"
)

type StatementType string

const (
	statementInsert       StatementType = "insert"
	statementCreateTable  StatementType = "create_table"
	statementDropTable    StatementType = "drop_table"
	statementLockTables   StatementType = "lock_tables"
	statementUnlockTables StatementType = "unlock_tables"
)

type statementInfo struct {
	typ    StatementType
	tables []string
	block  bool
}

type backend interface {
	compileAction(action string) (map[StatementType]struct{}, error)
	detectStatement(line []byte) (statementInfo, bool)
}

func resolveBackend(driver string) (backend, error) {
	switch strings.ToLower(strings.TrimSpace(driver)) {
	case "mysql":
		return mysqlBackend{}, nil
	default:
		return nil, fmt.Errorf("unsupported DB_DRIVER %q", driver)
	}
}
