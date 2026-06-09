package satverbalpolicy

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	sqldb "warwick-institute/internal/db"
)

type Target struct {
	Section string `json:"section"`
	Subject string `json:"subject"`
}

type RulePriority struct {
	Level           int                 `json:"level"`
	RuleType        string              `json:"ruleType"`
	Label           string              `json:"label"`
	MakeupTargets   []Target            `json:"makeupTargets"`
	SectionTargets  map[string][]Target `json:"sectionTargets"`
	EligibleTargets []string            `json:"eligibleTargets"`
	AnyDay          bool                `json:"anyDay"`
}

type CourseRule struct {
	ID                string              `json:"id"`
	CourseName        string              `json:"courseName"`
	Subject           string              `json:"subject"`
	RuleType          string              `json:"ruleType"`
	PriorityCount     int                 `json:"priorityCount"`
	Description       string              `json:"description"`
	MakeupRules       []string            `json:"makeupRules"`
	LastClassExcluded bool                `json:"lastClassExcluded"`
	MakeupTargets     []Target            `json:"makeupTargets"`
	SectionTargets    map[string][]Target `json:"sectionTargets"`
	EligibleTargets   []string            `json:"eligibleTargets"`
	Priorities        []RulePriority      `json:"priorities"`
}

type MatchedCourseReport struct {
	PolicyCourseName string `json:"policy_course_name"`
	CourseID         string `json:"course_id"`
	CourseCode       string `json:"course_code"`
	CourseName       string `json:"course_name"`
	RootGroupName    string `json:"root_group_name"`
}

type ApplyReport struct {
	Warnings            []string              `json:"warnings"`
	MatchedCourses      []MatchedCourseReport `json:"matched_courses"`
	UnmatchedPolicyRows []string              `json:"unmatched_policy_rows"`
	UnmatchedCourses    []string              `json:"unmatched_courses"`
}

var (
	nonWordRe = regexp.MustCompile(`[^a-z0-9]+`)
	spaceRe   = regexp.MustCompile(`\s+`)
	sectionRe = regexp.MustCompile(`\bsection\s*([0-9]+)\b`)
)

func DecodeRules(raw []byte) ([]CourseRule, error) {
	var rules []CourseRule
	if err := json.Unmarshal(raw, &rules); err != nil {
		return nil, err
	}
	for i, rule := range rules {
		if strings.TrimSpace(rule.ID) == "" || strings.TrimSpace(rule.CourseName) == "" {
			return nil, fmt.Errorf("policy row %d missing id or courseName", i+1)
		}
	}
	return rules, nil
}

func HashPolicy(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func NormalizeName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	name = strings.ReplaceAll(name, "rank 3-section", "rank 3 section")
	name = nonWordRe.ReplaceAllString(name, " ")
	name = spaceRe.ReplaceAllString(name, " ")
	return strings.TrimSpace(name)
}

func NameAliases(name string) []string {
	base := NormalizeName(name)
	aliases := []string{base}
	if strings.HasPrefix(base, "sat verbal ") {
		aliases = append(aliases, "sat "+strings.TrimPrefix(base, "sat verbal "))
	}
	if strings.HasPrefix(base, "sat ") && !strings.HasPrefix(base, "sat verbal ") {
		aliases = append(aliases, "sat verbal "+strings.TrimPrefix(base, "sat "))
	}
	return uniqueStrings(aliases)
}

func CourseMatchesRule(rule CourseRule, courseName string) bool {
	courseAliases := NameAliases(courseName)
	ruleAliases := NameAliases(rule.CourseName)
	for _, c := range courseAliases {
		for _, r := range ruleAliases {
			if c == r {
				return true
			}
			if IsSectionedCourse(c) && c == r+" "+ExtractSection(c) {
				return true
			}
		}
	}
	return false
}

func MatchingRule(rules []CourseRule, courseName string) *CourseRule {
	for i := range rules {
		if CourseMatchesRule(rules[i], courseName) {
			return &rules[i]
		}
	}
	return nil
}

func ExtractSection(name string) string {
	m := sectionRe.FindStringSubmatch(NormalizeName(name))
	if len(m) != 2 {
		return ""
	}
	return "section " + m[1]
}

func DisplaySection(name string) string {
	section := ExtractSection(name)
	if section == "" {
		return ""
	}
	parts := strings.Split(section, " ")
	return "Section " + parts[len(parts)-1]
}

func IsSectionedCourse(normalizedName string) bool {
	return ExtractSection(normalizedName) != ""
}

func FamilyName(name string) string {
	n := NormalizeName(name)
	if section := ExtractSection(n); section != "" {
		return strings.TrimSpace(strings.TrimSuffix(n, section))
	}
	return n
}

func RootGroupKey(courseName string) string {
	family := FamilyName(courseName)
	if strings.HasPrefix(family, "sat verbal rank 3") {
		return "SAT Verbal Rank 3"
	}
	return TitleFromNormalized(family)
}

func RootGroupName(subjectCode, key string) string {
	code := strings.TrimSpace(subjectCode)
	if code == "" {
		code = "SATV"
	}
	return fmt.Sprintf("SAT Verbal %s - %s", code, key)
}

func TitleFromNormalized(normalized string) string {
	words := strings.Fields(normalized)
	for i, word := range words {
		if word == "sat" {
			words[i] = "SAT"
			continue
		}
		if word == "ii" || word == "iii" || word == "iv" || word == "v" {
			words[i] = strings.ToUpper(word)
			continue
		}
		words[i] = strings.ToUpper(word[:1]) + word[1:]
	}
	return strings.Join(words, " ")
}

func BuildApplyReport(rules []CourseRule, courses []sqldb.SubjectCourseV2, subjectCode string) ApplyReport {
	var report ApplyReport
	matchedRows := make(map[string]bool)

	for _, course := range courses {
		rule := MatchingRule(rules, course.Name)
		if rule == nil {
			report.UnmatchedCourses = append(report.UnmatchedCourses, course.Name)
			continue
		}
		matchedRows[rule.CourseName] = true
		courseID, _ := UUIDString(course.ID)
		rootName := RootGroupName(subjectCode, RootGroupKey(course.Name))
		report.MatchedCourses = append(report.MatchedCourses, MatchedCourseReport{
			PolicyCourseName: rule.CourseName,
			CourseID:         courseID,
			CourseCode:       course.Code,
			CourseName:       course.Name,
			RootGroupName:    rootName,
		})
	}

	for _, rule := range rules {
		if !matchedRows[rule.CourseName] {
			report.UnmatchedPolicyRows = append(report.UnmatchedPolicyRows, rule.CourseName)
			report.Warnings = append(report.Warnings, "No course found for "+rule.CourseName)
		}
	}
	sort.Strings(report.Warnings)
	sort.Slice(report.MatchedCourses, func(i, j int) bool {
		return report.MatchedCourses[i].CourseName < report.MatchedCourses[j].CourseName
	})
	sort.Strings(report.UnmatchedPolicyRows)
	sort.Strings(report.UnmatchedCourses)
	return report
}

func UUIDString(u pgtype.UUID) (string, error) {
	if !u.Valid {
		return "", fmt.Errorf("invalid uuid")
	}
	return fmt.Sprintf("%x-%x-%x-%x-%x", u.Bytes[0:4], u.Bytes[4:6], u.Bytes[6:8], u.Bytes[8:10], u.Bytes[10:16]), nil
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}
