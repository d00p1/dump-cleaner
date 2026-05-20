package filter

import (
	"fmt"
	"io"
	"regexp"
)

type Stats struct {
	TotalLines    int
	FilteredLines int
}

type RuleDefinition struct {
	Action    string
	AllTables bool
	Tables    []string
}

type Rule struct {
	actions   map[StatementType]struct{}
	allTables bool
	tables    []*regexp.Regexp
}

func CompileRules(driver string, defs []RuleDefinition) ([]Rule, error) {
	if len(defs) == 0 {
		return nil, nil
	}
	backend, err := resolveBackend(driver)
	if err != nil {
		return nil, err
	}

	rules := make([]Rule, 0, len(defs))
	for _, def := range defs {
		actions, err := backend.compileAction(def.Action)
		if err != nil {
			return nil, err
		}

		rule := Rule{actions: actions, allTables: def.AllTables}
		if !def.AllTables {
			rule.tables = make([]*regexp.Regexp, 0, len(def.Tables))
			for _, pattern := range def.Tables {
				re, err := regexp.Compile(pattern)
				if err != nil {
					return nil, fmt.Errorf("invalid table pattern %q: %w", pattern, err)
				}
				rule.tables = append(rule.tables, re)
			}
		}
		rules = append(rules, rule)
	}

	return rules, nil
}

func actionSet(actions ...StatementType) map[StatementType]struct{} {
	set := make(map[StatementType]struct{}, len(actions))
	for _, action := range actions {
		set[action] = struct{}{}
	}
	return set
}

func SQLFilter(r io.Reader, w io.Writer, rules []Rule, maxLineBytes int) (Stats, error) {
	return runFilter(r, w, mysqlBackend{}, rules, maxLineBytes)
}

func InsertFilter(r io.Reader, w io.Writer, skipTables []string, maxLineBytes int) (Stats, error) {
	rules, err := CompileRules("mysql", []RuleDefinition{
		{Action: "ddl", Tables: append([]string(nil), skipTables...)},
		{Action: "insert", Tables: append([]string(nil), skipTables...)},
	})
	if err != nil {
		return Stats{}, err
	}
	return SQLFilter(r, w, rules, maxLineBytes)
}
