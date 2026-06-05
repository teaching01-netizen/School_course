package legacysync

import (
	"testing"
)

func TestClassroomName_WithIdAndName_ExtractsName(t *testing.T) {
	name := ExtractClassroomName("[120204] 12A: Auditorium (XL)")
	if name != "12A: Auditorium (XL)" {
		t.Errorf("expected '12A: Auditorium (XL)', got '%s'", name)
	}
}

func TestClassroomName_NotSet_ReturnsEmpty(t *testing.T) {
	name := ExtractClassroomName("[NOT SET]")
	if name != "" {
		t.Errorf("expected empty string, got '%s'", name)
	}
}

func TestClassroomName_NoBrackets_ReturnsRaw(t *testing.T) {
	name := ExtractClassroomName("Room 101")
	if name != "Room 101" {
		t.Errorf("expected 'Room 101', got '%s'", name)
	}
}

func TestMatchRoom_ExactMatch_Found(t *testing.T) {
	rooms := []Room{
		{ID: "room-1", Name: "Auditorium (XL)"},
		{ID: "room-2", Name: "Room 101"},
	}
	matched := MatchRoom("Auditorium (XL)", rooms)
	if matched == nil {
		t.Fatal("expected match, got nil")
	}
	if matched.ID != "room-1" {
		t.Errorf("expected room-1, got %s", matched.ID)
	}
}

func TestMatchRoom_PartialMatch_Found(t *testing.T) {
	rooms := []Room{
		{ID: "room-1", Name: "12A: Auditorium (XL)"},
	}
	matched := MatchRoom("Auditorium", rooms)
	if matched == nil {
		t.Fatal("expected partial match, got nil")
	}
	if matched.ID != "room-1" {
		t.Errorf("expected room-1, got %s", matched.ID)
	}
}

func TestMatchRoom_NoMatch_ReturnsNil(t *testing.T) {
	rooms := []Room{
		{ID: "room-1", Name: "Room 101"},
	}
	matched := MatchRoom("NonExistent Room", rooms)
	if matched != nil {
		t.Errorf("expected nil, got %v", matched)
	}
}

func TestMatchRoom_EmptyClassroom_ReturnsNil(t *testing.T) {
	rooms := []Room{
		{ID: "room-1", Name: "Room 101"},
	}
	matched := MatchRoom("", rooms)
	if matched != nil {
		t.Errorf("expected nil for empty classroom, got %v", matched)
	}
}
