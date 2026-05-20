package filter

func shouldSkipStatement(info statementInfo, rules []Rule) bool {
	for _, rule := range rules {
		if _, ok := rule.actions[info.typ]; !ok {
			continue
		}
		if rule.allTables {
			return true
		}
		for _, table := range info.tables {
			for _, pattern := range rule.tables {
				if pattern.MatchString(table) {
					return true
				}
			}
		}
	}
	return false
}
