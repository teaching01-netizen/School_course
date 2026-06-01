package xlsx

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/xuri/excelize/v2"
)

// Row represents a single row from the CRM XLSX export.
type Row struct {
	CycleLabel string
	CourseName string
	WCode      string

	FirstName           string
	LastName            string
	Nickname            string
	SecondarySchool     string
	AcademicLevel       string
	MobilePhone         string
	Hours               *int32
	TeachersRaw         string
	PrimaryEmail        string
	ParentName          string
	ParentPhone         string
	ParentEmail         string
	OrderQuoteUpdatedAt *time.Time
}

// Hash returns a deterministic hash for deduplication within a single upload.
func (r Row) Hash() string {
	parts := []string{
		strings.TrimSpace(r.WCode),
		strings.TrimSpace(r.CourseName),
		strings.TrimSpace(r.CycleLabel),
		strings.TrimSpace(r.TeachersRaw),
	}
	if r.Hours != nil {
		parts = append(parts, fmt.Sprintf("hours=%d", *r.Hours))
	}
	if r.OrderQuoteUpdatedAt != nil {
		parts = append(parts, "updated_at="+r.OrderQuoteUpdatedAt.UTC().Format(time.RFC3339Nano))
	}
	h := sha256.Sum256([]byte(strings.Join(parts, "|")))
	return hex.EncodeToString(h[:])
}

// ParsedXLSX contains the rows extracted from an XLSX file.
type ParsedXLSX struct {
	Rows []Row
}

var requiredHeaders = []string{
	"Student Id",
	"First Name",
	"Last Name",
	"Course Name",
	"Cycle",
}

// cleanPhoneSuffix strips trailing non-phone text from a CRM phone string.
// It scans left-to-right, keeping digits, +, -, (, ), and spaces.
// Once it encounters a disallowed character after collecting at least one digit,
// it stops and returns the collected portion (trimmed).
// If no digits are found, it returns an empty string.
func cleanPhoneSuffix(raw string) string {
	var out strings.Builder
	hadDigit := false
	for _, r := range raw {
		if unicode.IsDigit(r) || r == '+' || r == '-' || r == '(' || r == ')' || r == ' ' {
			if unicode.IsDigit(r) {
				hadDigit = true
			}
			out.WriteRune(r)
		} else if hadDigit {
			break
		}
	}
	if !hadDigit {
		return ""
	}
	return strings.TrimSpace(out.String())
}

// ParseXLSX parses an XLSX byte slice into rows using the canonical CRM schema.
func ParseXLSX(xlsxBytes []byte, instituteLoc *time.Location) (ParsedXLSX, error) {
	f, err := excelize.OpenReader(bytes.NewReader(xlsxBytes))
	if err != nil {
		return ParsedXLSX{}, fmt.Errorf("open xlsx: %w", err)
	}
	defer func() { _ = f.Close() }()

	sheets := f.GetSheetList()
	if len(sheets) == 0 {
		return ParsedXLSX{}, fmt.Errorf("xlsx has no sheets")
	}
	sheet := sheets[0]

	const scanRows = 20
	headerRowIdx := -1
	var headerCells []string

	allRows, err := f.GetRows(sheet, excelize.Options{RawCellValue: true})
	if err != nil {
		return ParsedXLSX{}, fmt.Errorf("read rows: %w", err)
	}

	for i := 0; i < len(allRows) && i < scanRows; i++ {
		cells := allRows[i]
		if looksLikeHeaderRow(cells) {
			headerRowIdx = i
			headerCells = cells
			break
		}
	}
	if headerRowIdx == -1 {
		return ParsedXLSX{}, fmt.Errorf("header row not found in first %d rows", scanRows)
	}

	colByHeader := map[string]int{}
	for idx, h := range headerCells {
		h = strings.TrimSpace(h)
		if h == "" {
			continue
		}
		colByHeader[h] = idx
	}
	for _, rh := range requiredHeaders {
		if _, ok := colByHeader[rh]; !ok {
			return ParsedXLSX{}, fmt.Errorf("missing required header %q", rh)
		}
	}

	var out []Row
	for i := headerRowIdx + 1; i < len(allRows); i++ {
		cells := allRows[i]

		get := func(header string) string {
			idx, ok := colByHeader[header]
			if !ok || idx < 0 || idx >= len(cells) {
				return ""
			}
			return strings.TrimSpace(cells[idx])
		}

		wcode := get("Student Id")
		courseName := get("Course Name")
		cycleLabel := get("Cycle")
		if wcode == "" && courseName == "" && cycleLabel == "" {
			continue
		}
		if wcode == "" || courseName == "" || cycleLabel == "" {
			continue
		}

		var hoursPtr *int32
		if s := get("Hours"); s != "" {
			if n, err := strconv.Atoi(strings.ReplaceAll(s, ",", "")); err == nil && n >= 0 {
				n32 := int32(n)
				hoursPtr = &n32
			}
		}

		var updatedAtPtr *time.Time
		if s := get("Order Quote Updated Date"); s != "" {
			if t, err := time.ParseInLocation("02/01/2006 03:04 PM", s, instituteLoc); err == nil {
				u := t.UTC()
				updatedAtPtr = &u
			} else if t, err := time.Parse(time.RFC3339, s); err == nil {
				u := t.UTC()
				updatedAtPtr = &u
			}
		}

		r := Row{
			CycleLabel: cycleLabel,
			CourseName: courseName,
			WCode:      wcode,

			FirstName:           get("First Name"),
			LastName:            get("Last Name"),
			Nickname:            get("Nickname"),
			SecondarySchool:     get("Secondary School"),
			AcademicLevel:       get("Academic level"),
			MobilePhone:         cleanPhoneSuffix(get("Mobile Phone")),
			Hours:               hoursPtr,
			TeachersRaw:         get("Teacher(s)"),
			PrimaryEmail:        get("Primary E-mail"),
			ParentName:          get("Parent Name"),
			ParentPhone:         cleanPhoneSuffix(get("phoneparent")),
			ParentEmail:         get("emailparent"),
			OrderQuoteUpdatedAt: updatedAtPtr,
		}
		out = append(out, r)
	}

	if len(out) == 0 {
		return ParsedXLSX{}, fmt.Errorf("xlsx contained 0 usable data rows")
	}
	return ParsedXLSX{Rows: out}, nil
}

func looksLikeHeaderRow(cells []string) bool {
	need := map[string]bool{"Student Id": false, "Course Name": false, "Cycle": false}
	for _, c := range cells {
		c = strings.TrimSpace(c)
		if _, ok := need[c]; ok {
			need[c] = true
		}
	}
	for _, ok := range need {
		if !ok {
			return false
		}
	}
	return true
}
