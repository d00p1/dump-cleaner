package filter

import (
	"bytes"
	"strings"
	"testing"
)

func TestInsertFilterLargeLines(t *testing.T) {
	longValue := strings.Repeat("x", 2*1024*1024)
	input := "CREATE TABLE `tmp_log` (id int);\n" +
		"INSERT INTO `tmp_log` VALUES (1, '" + longValue + "');\n" +
		"CREATE TABLE `users` (id int);\n"

	var out bytes.Buffer
	stats, err := InsertFilter(strings.NewReader(input), &out, []string{"^tmp_"}, 4*1024*1024)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if strings.Contains(out.String(), "INSERT INTO `tmp_log`") {
		t.Fatalf("tmp_log insert should be removed")
	}
	if !strings.Contains(out.String(), "CREATE TABLE `users`") {
		t.Fatalf("users DDL should remain")
	}
	if stats.FilteredLines == 0 {
		t.Fatalf("expected filtered lines > 0")
	}
}

func TestInsertFilterRemovesCreateDropAndInsertForSkippedTables(t *testing.T) {
	input := "DROP TABLE IF EXISTS `b_sale_basket_tmp`;\n" +
		"CREATE TABLE `b_sale_basket_tmp` (\n" +
		"  `id` int\n" +
		");\n" +
		"INSERT INTO `b_sale_basket_tmp` VALUES\n" +
		"(1),\n" +
		"(2);\n" +
		"CREATE TABLE `users` (id int);\n"

	var out bytes.Buffer
	stats, err := InsertFilter(strings.NewReader(input), &out, []string{"^b_.+_tmp$"}, 1024)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	result := out.String()
	if strings.Contains(result, "b_sale_basket_tmp") {
		t.Fatalf("skipped table statements should be removed, got %q", result)
	}
	if !strings.Contains(result, "CREATE TABLE `users`") {
		t.Fatalf("non-skipped table DDL should remain")
	}
	if stats.FilteredLines != 7 {
		t.Fatalf("expected 7 filtered lines, got %d", stats.FilteredLines)
	}
}

func TestInsertFilterFailsWhenLineLimitExceeded(t *testing.T) {
	longValue := strings.Repeat("x", 2048)
	input := "INSERT INTO `tmp_log` VALUES ('" + longValue + "');\n"

	_, err := InsertFilter(strings.NewReader(input), &bytes.Buffer{}, []string{"^tmp_"}, 1024)
	if err == nil {
		t.Fatalf("expected line-limit error")
	}
}

func TestSQLFilterSupportsTablesAllAndLocksAlias(t *testing.T) {
	rules, err := CompileRules("mysql", []RuleDefinition{{Action: "locks", AllTables: true}})
	if err != nil {
		t.Fatalf("compile rules: %v", err)
	}

	input := "LOCK TABLES `users` WRITE, `orders` READ;\n" +
		"UNLOCK TABLES;\n" +
		"INSERT INTO `users` VALUES (1);\n"

	var out bytes.Buffer
	stats, err := SQLFilter(strings.NewReader(input), &out, rules, 1024)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	result := out.String()
	if strings.Contains(result, "LOCK TABLES") || strings.Contains(result, "UNLOCK TABLES") {
		t.Fatalf("lock statements should be removed, got %q", result)
	}
	if !strings.Contains(result, "INSERT INTO `users`") {
		t.Fatalf("non-lock statements should remain")
	}
	if stats.FilteredLines != 2 {
		t.Fatalf("expected 2 filtered lines, got %d", stats.FilteredLines)
	}
}

func TestCompileRulesRejectsUnknownAction(t *testing.T) {
	_, err := CompileRules("mysql", []RuleDefinition{{Action: "truncate", AllTables: true}})
	if err == nil {
		t.Fatalf("expected unknown action error")
	}
}

func TestCompileRulesRejectsUnknownDriver(t *testing.T) {
	_, err := CompileRules("postgres", []RuleDefinition{{Action: "all", AllTables: true}})
	if err == nil {
		t.Fatalf("expected unknown driver error")
	}
}

func TestMySQLBackendDetectsMultiTableDrop(t *testing.T) {
	info, ok := mysqlBackend{}.detectStatement([]byte("DROP TABLE IF EXISTS `tmp_log`, `tmp_cache`;"))
	if !ok {
		t.Fatalf("expected statement detection")
	}
	if info.typ != statementDropTable {
		t.Fatalf("unexpected statement type: %q", info.typ)
	}
	if len(info.tables) != 2 || info.tables[0] != "tmp_log" || info.tables[1] != "tmp_cache" {
		t.Fatalf("unexpected tables: %#v", info.tables)
	}
}

func TestMySQLBackendDetectsLockTables(t *testing.T) {
	info, ok := mysqlBackend{}.detectStatement([]byte("LOCK TABLES `users` WRITE, `orders` READ;"))
	if !ok {
		t.Fatalf("expected statement detection")
	}
	if info.typ != statementLockTables {
		t.Fatalf("unexpected statement type: %q", info.typ)
	}
	if len(info.tables) != 2 || info.tables[0] != "users" || info.tables[1] != "orders" {
		t.Fatalf("unexpected tables: %#v", info.tables)
	}
}

func TestInsertFilterMatchesEquivalentCompiledRules(t *testing.T) {
	input := "DROP TABLE IF EXISTS `tmp_log`;\n" +
		"CREATE TABLE `tmp_log` (id int);\n" +
		"INSERT INTO `tmp_log` VALUES (1);\n" +
		"CREATE TABLE `users` (id int);\n"

	var legacyOut bytes.Buffer
	legacyStats, err := InsertFilter(strings.NewReader(input), &legacyOut, []string{"^tmp_"}, 1024)
	if err != nil {
		t.Fatalf("legacy filter failed: %v", err)
	}

	rules, err := CompileRules("mysql", []RuleDefinition{
		{Action: "ddl", Tables: []string{"^tmp_"}},
		{Action: "insert", Tables: []string{"^tmp_"}},
	})
	if err != nil {
		t.Fatalf("compile rules: %v", err)
	}

	var rulesOut bytes.Buffer
	rulesStats, err := SQLFilter(strings.NewReader(input), &rulesOut, rules, 1024)
	if err != nil {
		t.Fatalf("rule-based filter failed: %v", err)
	}

	if legacyOut.String() != rulesOut.String() {
		t.Fatalf("legacy and rule-based output differ:\nlegacy=%q\nrules=%q", legacyOut.String(), rulesOut.String())
	}
	if legacyStats != rulesStats {
		t.Fatalf("legacy and rule-based stats differ: legacy=%#v rules=%#v", legacyStats, rulesStats)
	}
}
