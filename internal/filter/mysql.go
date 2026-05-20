package filter

import (
	"fmt"
	"regexp"
	"strings"
)

type mysqlBackend struct{}

var (
	reInsert = regexp.MustCompile("(?i)^INSERT\\s+INTO\\s+(?:`[^`]+`\\.)?`?([^` ]+)`?")
	reCreate = regexp.MustCompile("(?i)^CREATE\\s+(?:TEMPORARY\\s+)?TABLE\\s+(?:IF\\s+NOT\\s+EXISTS\\s+)?(?:`[^`]+`\\.)?`?([^` (]+)`?")
	reDrop   = regexp.MustCompile("(?i)^DROP\\s+(?:TEMPORARY\\s+)?TABLE\\s+(?:IF\\s+EXISTS\\s+)?(.+?);?$")
	reLock   = regexp.MustCompile("(?i)^LOCK\\s+TABLES\\s+(.+?);?$")
	reUnlock = regexp.MustCompile("(?i)^UNLOCK\\s+TABLES\\b")

	reDropEntry = regexp.MustCompile("(?:^|,)\\s*(?:`[^`]+`\\.)?`?([^`,\\s]+)`?")
	reLockEntry = regexp.MustCompile("(?:^|,)\\s*(?:`[^`]+`\\.)?`?([^`,\\s]+)`?\\s+(?:READ|WRITE)\\b")
)

func (mysqlBackend) compileAction(action string) (map[StatementType]struct{}, error) {
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "insert":
		return actionSet(statementInsert), nil
	case "create_table":
		return actionSet(statementCreateTable), nil
	case "drop_table":
		return actionSet(statementDropTable), nil
	case "ddl":
		return actionSet(statementCreateTable, statementDropTable), nil
	case "locks":
		return actionSet(statementLockTables, statementUnlockTables), nil
	case "all":
		return actionSet(statementInsert, statementCreateTable, statementDropTable, statementLockTables, statementUnlockTables), nil
	default:
		return nil, fmt.Errorf("unsupported filter action %q for mysql", action)
	}
}

func (mysqlBackend) detectStatement(line []byte) (statementInfo, bool) {
	if matches := reInsert.FindSubmatch(line); matches != nil {
		return statementInfo{typ: statementInsert, tables: []string{string(matches[1])}, block: true}, true
	}
	if matches := reCreate.FindSubmatch(line); matches != nil {
		return statementInfo{typ: statementCreateTable, tables: []string{string(matches[1])}, block: true}, true
	}
	if matches := reDrop.FindSubmatch(line); matches != nil {
		return statementInfo{typ: statementDropTable, tables: extractTables(matches[1], reDropEntry)}, true
	}
	if matches := reLock.FindSubmatch(line); matches != nil {
		return statementInfo{typ: statementLockTables, tables: extractTables(matches[1], reLockEntry)}, true
	}
	if reUnlock.Match(line) {
		return statementInfo{typ: statementUnlockTables}, true
	}
	return statementInfo{}, false
}

func extractTables(fragment []byte, entryRE *regexp.Regexp) []string {
	matches := entryRE.FindAllSubmatch(fragment, -1)
	tables := make([]string, 0, len(matches))
	for _, match := range matches {
		tables = append(tables, string(match[1]))
	}
	return tables
}
