package xlsx

import "testing"

func TestRowHashIncludesStudentIdentityAndContactFields(t *testing.T) {
	base := Row{
		WCode:        "W250001",
		CourseName:   "Math",
		CycleLabel:   "Cycle A",
		FirstName:    "Alice",
		LastName:     "Alpha",
		PrimaryEmail: "alice@example.com",
		MobilePhone:  "081-111-1111",
	}

	for name, mutate := range map[string]func(*Row){
		"first name": func(r *Row) { r.FirstName = "Alicia" },
		"last name":  func(r *Row) { r.LastName = "Beta" },
		"email":      func(r *Row) { r.PrimaryEmail = "other@example.com" },
		"phone":      func(r *Row) { r.MobilePhone = "081-222-2222" },
	} {
		t.Run(name, func(t *testing.T) {
			changed := base
			mutate(&changed)
			if changed.Hash() == base.Hash() {
				t.Fatalf("Hash did not change when %s changed", name)
			}
		})
	}
}
