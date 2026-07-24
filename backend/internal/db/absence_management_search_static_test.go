package db

import (
	"strings"
	"testing"
)

func TestManagedAbsenceListSearchIncludesStudentNickname(t *testing.T) {
	for name, hasStudentNicknameColumn := range map[string]bool{
		"legacy schema":  false,
		"current schema": true,
	} {
		t.Run(name, func(t *testing.T) {
			sql := managedAbsenceQuerySQL(managedAbsenceListQueryTemplate, hasStudentNicknameColumn)
			nicknameExpr := "st.nickname"
			if hasStudentNicknameColumn {
				nicknameExpr = "COALESCE(sa.student_nickname, st.nickname)"
			}
			searchPredicate := "COALESCE(" + nicknameExpr + ", '') ILIKE '%' || $1 || '%'"
			if !strings.Contains(sql, searchPredicate) {
				t.Fatalf("absence inbox search must include the resolved student nickname, got SQL:\n%s", sql)
			}
		})
	}
}
