package currentformmodel

import (
	"regexp"
	"strings"
	"testing"
	"time"
)

// TestPatternCronIsRE2Safe proves the structural half of the grammar compiles
// under Go's RE2 engine, which is the same engine the Terraform schema, the
// Draft 2020-12 desired schema, and a conforming host all use.
func TestPatternCronIsRE2Safe(t *testing.T) {
	if _, err := regexp.Compile(PatternCron); err != nil {
		t.Fatalf("PatternCron does not compile under RE2: %v", err)
	}
}

// TestCronAcceptsHourlyAndSubHourlySchedules is the regression the whole
// grammar exists for: the previous pattern rejected every schedule more
// frequent than daily, which made a Form whose purpose is periodic invocation
// unable to express its most common use.
func TestCronAcceptsHourlyAndSubHourlySchedules(t *testing.T) {
	structural := regexp.MustCompile(PatternCron)
	for _, expression := range []string{
		"* * * * *",
		"0 * * * *",
		"*/5 * * * *",
		"0,30 * * * *",
		"15 0 * * *",
		"0 3 * * *",
		"0 9-17 * * 1-5",
		"0 0 1 1 *",
		"0-59/15 0-23/2 1-31 1-12 0-6",
	} {
		if !structural.MatchString(expression) {
			t.Fatalf("PatternCron rejected %q", expression)
		}
		if err := ValidateCron(expression); err != nil {
			t.Fatalf("ParseCron rejected %q: %v", expression, err)
		}
	}
}

// TestCronRejectsShapesThatAreNotSchedules proves the semantic half has teeth:
// each of these matches the structural pattern (or is refused by it) and none
// of them names a schedule.
func TestCronRejectsShapesThatAreNotSchedules(t *testing.T) {
	for _, testCase := range []struct {
		expression string
		contains   string
	}{
		{"0 24 * * *", "hour value"},
		{"60 0 * * *", "minute value"},
		{"0 0 0 * *", "day-of-month value"},
		{"0 0 * 13 *", "month value"},
		{"0 0 * * 7", "day-of-week value"},
		{"5-1 * * * *", "inverted"},
		{"*/0 * * * *", "step"},
		{"*/60 * * * *", "step"},
		{"5/10 * * * *", "single value"},
		{"0 3 * *", "exactly 5 fields"},
		{"0 3 * * * *", "exactly 5 fields"},
		{"0  3 * * *", "exactly 5 fields"},
		{"0,,3 * * * *", "empty term"},
		{"", "required"},
	} {
		err := ValidateCron(testCase.expression)
		if err == nil {
			t.Fatalf("ParseCron accepted %q", testCase.expression)
		}
		if !strings.Contains(err.Error(), testCase.contains) {
			t.Fatalf("ParseCron(%q) said %q, which does not name %q", testCase.expression, err, testCase.contains)
		}
	}
}

// TestCronCanonicalFormIdentifiesEqualSchedules proves two spellings of one
// schedule canonicalise to one string, and that the canonical form of a whole
// domain is `*`.
func TestCronCanonicalFormIdentifiesEqualSchedules(t *testing.T) {
	for _, testCase := range []struct{ expression, canonical string }{
		{"* * * * *", "* * * * *"},
		{"*/1 * * * *", "* * * * *"},
		{"0-59 0-23 1-31 1-12 0-6", "* * * * *"},
		{"*/15 * * * *", "0,15,30,45 * * * *"},
		{"45,0,15,30 * * * *", "0,15,30,45 * * * *"},
		{"0 9-17 * * 1-5", "0 9-17 * * 1-5"},
		{"0 0,1,2,5 * * *", "0 0-2,5 * * *"},
		{"05 03 * * *", "5 3 * * *"},
	} {
		schedule, err := ParseCron(testCase.expression)
		if err != nil {
			t.Fatalf("ParseCron(%q): %v", testCase.expression, err)
		}
		if got := schedule.Canonical(); got != testCase.canonical {
			t.Fatalf("ParseCron(%q).Canonical() = %q, want %q", testCase.expression, got, testCase.canonical)
		}
	}
}

// TestCronDayCombinationRuleIsTheOrRule proves the one place cron is not a
// conjunction. With both day fields restricted the schedule fires when EITHER
// matches; with one restricted only that one constrains.
func TestCronDayCombinationRuleIsTheOrRule(t *testing.T) {
	// 2026-08-03 is a Monday; 2026-08-10 is the following Monday.
	monday := time.Date(2026, time.August, 3, 3, 0, 0, 0, time.UTC)
	firstOfMonth := time.Date(2026, time.August, 1, 3, 0, 0, 0, time.UTC)
	tuesdayThird := time.Date(2026, time.August, 4, 3, 0, 0, 0, time.UTC)

	both, err := ParseCron("0 3 1 * 1")
	if err != nil {
		t.Fatalf("ParseCron: %v", err)
	}
	if !both.Matches(monday) {
		t.Fatal("both-restricted schedule did not match the day-of-week arm")
	}
	if !both.Matches(firstOfMonth) {
		t.Fatal("both-restricted schedule did not match the day-of-month arm")
	}
	if both.Matches(tuesdayThird) {
		t.Fatal("both-restricted schedule matched a day neither arm selects")
	}

	dayOfMonthOnly, err := ParseCron("0 3 1 * *")
	if err != nil {
		t.Fatalf("ParseCron: %v", err)
	}
	if dayOfMonthOnly.Matches(monday) {
		t.Fatal("a day-of-month-only schedule matched a day it does not select")
	}
	if !dayOfMonthOnly.Matches(firstOfMonth) {
		t.Fatal("a day-of-month-only schedule missed its own day")
	}

	dayOfWeekOnly, err := ParseCron("0 3 * * 1")
	if err != nil {
		t.Fatalf("ParseCron: %v", err)
	}
	if !dayOfWeekOnly.Matches(monday) {
		t.Fatal("a day-of-week-only schedule missed its own day")
	}
	if dayOfWeekOnly.Matches(tuesdayThird) {
		t.Fatal("a day-of-week-only schedule matched another weekday")
	}
}

// TestCronMatchesInterpretsEveryInstantInUTC proves the contract's one
// timezone rule: an instant carrying another zone is converted, never read in
// its own wall clock.
func TestCronMatchesInterpretsEveryInstantInUTC(t *testing.T) {
	schedule, err := ParseCron("0 3 * * *")
	if err != nil {
		t.Fatalf("ParseCron: %v", err)
	}
	zone := time.FixedZone("UTC+9", 9*60*60)
	// 12:00 in UTC+9 is 03:00 UTC and matches; 03:00 in UTC+9 is 18:00 UTC and
	// does not.
	if !schedule.Matches(time.Date(2026, time.August, 3, 12, 0, 0, 0, zone)) {
		t.Fatal("a matching instant expressed in another zone was missed")
	}
	if schedule.Matches(time.Date(2026, time.August, 3, 3, 0, 0, 0, zone)) {
		t.Fatal("a non-matching instant was matched by reading its local wall clock")
	}
}
