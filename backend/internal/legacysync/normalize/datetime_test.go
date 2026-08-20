package normalize

import (
	"testing"
	"time"
)

func mustLoadLocation(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Fatalf("LoadLocation(%q): %v", name, err)
	}
	return loc
}

func TestParseLegacyDate(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    time.Time // UTC; zero value means error expected
		wantErr bool
	}{
		{"weekday prefix", "Sat 23 May 26", time.Date(2026, 5, 23, 0, 0, 0, 0, time.UTC), false},
		{"weekday prefix other", "Sun 24 May 26", time.Date(2026, 5, 24, 0, 0, 0, 0, time.UTC), false},
		{"no weekday", "23 May 26", time.Date(2026, 5, 23, 0, 0, 0, 0, time.UTC), false},
		{"english short month 4-digit year", "23 May 2026", time.Date(2026, 5, 23, 0, 0, 0, 0, time.UTC), false},
		{"english other month", "15 Jan 26", time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC), false},
		{"english full month", "23 December 2026", time.Date(2026, 12, 23, 0, 0, 0, 0, time.UTC), false},
		{"numeric dd mm yy", "23/05/26", time.Date(2026, 5, 23, 0, 0, 0, 0, time.UTC), false},
		{"numeric dd mm yyyy", "23/05/2026", time.Date(2026, 5, 23, 0, 0, 0, 0, time.UTC), false},
		{"numeric be year", "23/05/2569", time.Date(2026, 5, 23, 0, 0, 0, 0, time.UTC), false},
		{"numeric single digit day month", "3/5/26", time.Date(2026, 5, 3, 0, 0, 0, 0, time.UTC), false},
		{"thai short jan", "15 ม.ค. 2569", time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC), false},
		{"thai short feb", "15 ก.พ. 2569", time.Date(2026, 2, 15, 0, 0, 0, 0, time.UTC), false},
		{"thai short mar", "15 มี.ค. 2569", time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC), false},
		{"thai short apr", "15 เม.ย. 2569", time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC), false},
		{"thai short may", "23 พ.ค. 2569", time.Date(2026, 5, 23, 0, 0, 0, 0, time.UTC), false},
		{"thai short jun", "15 มิ.ย. 2569", time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC), false},
		{"thai short jul", "15 ก.ค. 2569", time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC), false},
		{"thai short aug", "15 ส.ค. 2569", time.Date(2026, 8, 15, 0, 0, 0, 0, time.UTC), false},
		{"thai short sep", "15 ก.ย. 2569", time.Date(2026, 9, 15, 0, 0, 0, 0, time.UTC), false},
		{"thai short oct", "15 ต.ค. 2569", time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC), false},
		{"thai short nov", "15 พ.ย. 2569", time.Date(2026, 11, 15, 0, 0, 0, 0, time.UTC), false},
		{"thai short dec", "15 ธ.ค. 2569", time.Date(2026, 12, 15, 0, 0, 0, 0, time.UTC), false},
		{"thai full jan", "15 มกราคม 2569", time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC), false},
		{"thai full feb", "15 กุมภาพันธ์ 2569", time.Date(2026, 2, 15, 0, 0, 0, 0, time.UTC), false},
		{"thai full mar", "15 มีนาคม 2569", time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC), false},
		{"thai full apr", "15 เมษายน 2569", time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC), false},
		{"thai full may", "23 พฤษภาคม 2569", time.Date(2026, 5, 23, 0, 0, 0, 0, time.UTC), false},
		{"thai full jun", "15 มิถุนายน 2569", time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC), false},
		{"thai full jul", "15 กรกฎาคม 2569", time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC), false},
		{"thai full aug", "15 สิงหาคม 2569", time.Date(2026, 8, 15, 0, 0, 0, 0, time.UTC), false},
		{"thai full sep", "15 กันยายน 2569", time.Date(2026, 9, 15, 0, 0, 0, 0, time.UTC), false},
		{"thai full oct", "15 ตุลาคม 2569", time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC), false},
		{"thai full nov", "15 พฤศจิกายน 2569", time.Date(2026, 11, 15, 0, 0, 0, 0, time.UTC), false},
		{"thai full dec", "15 ธันวาคม 2569", time.Date(2026, 12, 15, 0, 0, 0, 0, time.UTC), false},
		{"thai short with leading space", "  23 พ.ค. 2569  ", time.Date(2026, 5, 23, 0, 0, 0, 0, time.UTC), false},
		// Invalid inputs.
		{"empty", "", time.Time{}, true},
		{"day out of range", "32/13/26", time.Time{}, true},
		{"month out of range", "23/13/26", time.Time{}, true},
		{"day zero", "0/05/26", time.Time{}, true},
		{"feb 30", "30/02/26", time.Time{}, true},
		{"garbage", "abc", time.Time{}, true},
		{"wrong separator", "23-05-26", time.Time{}, true},
		{"unknown month", "23 Foo 26", time.Time{}, true},
		{"day not numeric in word form", "Sat May 26", time.Time{}, true},
		{"trailing garbage after date", "23 May 26 extra", time.Time{}, true},
		{"year out of band low", "23/05/1950", time.Time{}, true},
		{"be year out of band", "23/05/2500", time.Time{}, true},
		{"year out of band high", "23/05/3000", time.Time{}, true},
		{"three digit year", "23/05/999", time.Time{}, true},
		{"negative day", "-5/05/26", time.Time{}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseLegacyDate(tt.in)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ParseLegacyDate(%q) = %v, want error", tt.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseLegacyDate(%q) unexpected error: %v", tt.in, err)
			}
			if !got.Equal(tt.want) {
				t.Errorf("ParseLegacyDate(%q) = %v, want %v", tt.in, got, tt.want)
			}
			if got.Location() != time.UTC {
				t.Errorf("ParseLegacyDate(%q) location = %v, want UTC", tt.in, got.Location())
			}
		})
	}
}

func TestParseClock(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    int
		wantErr bool
	}{
		{"eleven o clock", "11:00", 660, false},
		{"single digit hour", "9:05", 545, false},
		{"zero padded hour", "09:05", 545, false},
		{"midnight", "00:00", 0, false},
		{"single digit hour zero", "0:00", 0, false},
		{"end of day", "23:59", 1439, false},
		{"empty", "", 0, true},
		{"hour out of range", "25:00", 0, true},
		{"minute out of range", "11:60", 0, true},
		{"garbage", "abc", 0, true},
		{"minute single digit", "11:0", 0, true},
		{"minute and hour single digit", "5:5", 0, true},
		{"three part", "11:00:00", 0, true},
		{"missing minutes", "11", 0, true},
		{"missing minutes colon", "11:", 0, true},
		{"leading space", " 9:05", 0, true},
		{"plus sign hour", "+9:05", 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseClock(tt.in)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ParseClock(%q) = %d, want error", tt.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseClock(%q) unexpected error: %v", tt.in, err)
			}
			if got != tt.want {
				t.Errorf("ParseClock(%q) = %d, want %d", tt.in, got, tt.want)
			}
		})
	}
}

func TestSessionWindow(t *testing.T) {
	bangkok := mustLoadLocation(t, "Asia/Bangkok")
	tests := []struct {
		name      string
		date      time.Time
		begin     string
		end       string
		wantStart time.Time
		wantEnd   time.Time
		wantErr   bool
	}{
		{"same day morning", time.Date(2026, 5, 23, 0, 0, 0, 0, bangkok), "11:00", "13:00", time.Date(2026, 5, 23, 4, 0, 0, 0, time.UTC), time.Date(2026, 5, 23, 6, 0, 0, 0, time.UTC), false},
		{"crosses midnight", time.Date(2026, 5, 23, 0, 0, 0, 0, bangkok), "23:00", "01:00", time.Date(2026, 5, 23, 16, 0, 0, 0, time.UTC), time.Date(2026, 5, 23, 18, 0, 0, 0, time.UTC), false},
		{"reverse order is next day", time.Date(2026, 5, 23, 0, 0, 0, 0, bangkok), "13:00", "11:00", time.Date(2026, 5, 23, 6, 0, 0, 0, time.UTC), time.Date(2026, 5, 24, 4, 0, 0, 0, time.UTC), false},
		{"begin equals end", time.Date(2026, 5, 23, 0, 0, 0, 0, bangkok), "11:00", "11:00", time.Time{}, time.Time{}, true},
		{"invalid begin", time.Date(2026, 5, 23, 0, 0, 0, 0, bangkok), "25:00", "13:00", time.Time{}, time.Time{}, true},
		{"invalid end", time.Date(2026, 5, 23, 0, 0, 0, 0, bangkok), "11:00", "11:60", time.Time{}, time.Time{}, true},
		{"date clock fields do not leak", time.Date(2026, 5, 23, 23, 59, 59, 999, time.FixedZone("weird", -3600)), "11:00", "13:00", time.Date(2026, 5, 23, 4, 0, 0, 0, time.UTC), time.Date(2026, 5, 23, 6, 0, 0, 0, time.UTC), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start, end, err := SessionWindow(tt.date, tt.begin, tt.end, bangkok)
			if tt.wantErr {
				if err == nil {
					t.Errorf("SessionWindow(%q, %q) = (%v, %v), want error", tt.begin, tt.end, start, end)
				}
				return
			}
			if err != nil {
				t.Fatalf("SessionWindow(%q, %q) unexpected error: %v", tt.begin, tt.end, err)
			}
			if !start.Equal(tt.wantStart) {
				t.Errorf("SessionWindow(%q, %q) start = %v, want %v", tt.begin, tt.end, start, tt.wantStart)
			}
			if !end.Equal(tt.wantEnd) {
				t.Errorf("SessionWindow(%q, %q) end = %v, want %v", tt.begin, tt.end, end, tt.wantEnd)
			}
			if start.Location() != time.UTC || end.Location() != time.UTC {
				t.Errorf("SessionWindow(%q, %q) locations = %v, %v; want UTC", tt.begin, tt.end, start.Location(), end.Location())
			}
		})
	}
}

func TestLocalToUTC(t *testing.T) {
	bangkok := mustLoadLocation(t, "Asia/Bangkok")
	tests := []struct {
		name    string
		date    time.Time
		clock   string
		want    time.Time
		wantErr bool
	}{
		{"midnight boundary", time.Date(2026, 8, 3, 0, 0, 0, 0, bangkok), "00:00", time.Date(2026, 8, 2, 17, 0, 0, 0, time.UTC), false},
		{"noon", time.Date(2026, 8, 3, 0, 0, 0, 0, bangkok), "12:00", time.Date(2026, 8, 3, 5, 0, 0, 0, time.UTC), false},
		{"date clock fields do not leak", time.Date(2026, 8, 3, 23, 59, 59, 999, time.FixedZone("weird", -3600)), "00:00", time.Date(2026, 8, 2, 17, 0, 0, 0, time.UTC), false},
		{"invalid clock", time.Date(2026, 8, 3, 0, 0, 0, 0, bangkok), "25:00", time.Time{}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := LocalToUTC(tt.date, tt.clock, bangkok)
			if tt.wantErr {
				if err == nil {
					t.Errorf("LocalToUTC(%v, %q) = %v, want error", tt.date, tt.clock, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("LocalToUTC(%v, %q) unexpected error: %v", tt.date, tt.clock, err)
			}
			if !got.Equal(tt.want) {
				t.Errorf("LocalToUTC(%v, %q) = %v, want %v", tt.date, tt.clock, got, tt.want)
			}
			if got.Location() != time.UTC {
				t.Errorf("LocalToUTC(%v, %q) location = %v, want UTC", tt.date, tt.clock, got.Location())
			}
		})
	}
}

func FuzzParseLegacyDate(f *testing.F) {
	for _, seed := range []string{
		"Sat 23 May 26",
		"23 May 26",
		"23/05/26",
		"23/05/2026",
		"23/05/2569",
		"23 พ.ค. 2569",
		"23 พฤษภาคม 2569",
		"",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, s string) {
		got, err := ParseLegacyDate(s)
		if err != nil {
			return
		}
		if got.Year() < 1970 || got.Year() > 2100 {
			t.Fatalf("ParseLegacyDate(%q) = %v: year %d out of sane range 1970..2100", s, got, got.Year())
		}
		if got.Month() < 1 || got.Month() > 12 || got.Day() < 1 || got.Day() > 31 {
			t.Fatalf("ParseLegacyDate(%q) = %v: month/day out of range", s, got)
		}
		if got.Location() != time.UTC {
			t.Fatalf("ParseLegacyDate(%q) location = %v, want UTC", s, got.Location())
		}
	})
}
