package currentformmodel

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// cron.go is the portable five-field UTC cron grammar of the family lane
// ([decision 0020](../../spec/decisions/0020-the-edge-interfaces-state-their-data-and-delivery-model.md)).
//
// It exists because a regular expression alone cannot decide a schedule. The
// previous grammar was a regex and nothing else, and it rejected `* * * * *`,
// `0 * * * *`, and `*/5 * * * *` — every hourly and sub-hourly schedule — so a
// Form whose entire purpose is periodic invocation could express only "once a
// day at a fixed minute". Widening the regex alone would have been worse: a
// regex that admits `5-1`, `0-99`, or `*/0` admits nonsense, and two hosts
// would then disagree about which of those fire.
//
// So the grammar is stated twice, on purpose, and the two halves do different
// jobs. PatternCron is the STRUCTURAL half: RE2-safe, carried inside the
// published desired schema, and therefore enforceable by any host that has only
// the Form Definition. ParseCron is the SEMANTIC half: it builds the AST, holds
// every field to its own domain, and yields the canonical form and the match
// rule. A host, the provider's plan, and the conformance runner all call
// ParseCron, so all three refuse exactly the same expressions.
//
// Nothing here is timezone-aware by design: every expression is interpreted in
// UTC, so no two hosts can fire one trigger at different instants.

const (
	// cronTerm is one term of one field: `*`, `*/step`, a literal, a range, or
	// a range with a step. It is deliberately RE2-safe — non-capturing groups,
	// alternation, and bounded repetition only — so the same expression
	// constrains the Terraform schema, the Draft 2020-12 desired schema, and a
	// host's own validator without any of them needing another engine.
	cronTerm = `(?:\*(?:/[0-9]{1,2})?|[0-9]{1,2}(?:-[0-9]{1,2}(?:/[0-9]{1,2})?)?)`
	// cronFieldPattern is a comma-separated list of at least one term.
	cronFieldPattern = cronTerm + `(?:,` + cronTerm + `)*`
	// PatternCron is the structural five-field grammar: five term lists
	// separated by single spaces. It bounds the shape; ParseCron decides the
	// meaning, because a regex cannot know that `5-1` is inverted, that `0-99`
	// leaves the minute domain, or that `*/0` names no step at all.
	PatternCron = `^` + cronFieldPattern + `(?: ` + cronFieldPattern + `){4}$`
	// CronFieldCount is the exact number of fields; there is no seconds field
	// and no year field.
	CronFieldCount = 5
)

// cronFieldSpec is one field's domain.
type cronFieldSpec struct {
	name string
	min  int
	max  int
}

// cronFields is the ordered five-field domain table. Day-of-week is 0-6 with
// 0 = Sunday; there is no 7 alias and no name alias, because two spellings of
// one day are two ways for hosts to disagree.
var cronFields = [CronFieldCount]cronFieldSpec{
	{name: "minute", min: 0, max: 59},
	{name: "hour", min: 0, max: 23},
	{name: "day-of-month", min: 1, max: 31},
	{name: "month", min: 1, max: 12},
	{name: "day-of-week", min: 0, max: 6},
}

// CronSchedule is the parsed abstract syntax of one expression: five sorted,
// de-duplicated value sets plus the two flags the day rule needs.
type CronSchedule struct {
	Minute     []int
	Hour       []int
	DayOfMonth []int
	Month      []int
	DayOfWeek  []int

	// dayOfMonthRestricted and dayOfWeekRestricted record whether each day
	// field selects strictly fewer values than its whole domain. They decide
	// the combination rule below, and they are derived from the VALUE SET
	// rather than from the spelling, so `*`, `*/1`, and `1-31` are all
	// unrestricted day-of-month.
	dayOfMonthRestricted bool
	dayOfWeekRestricted  bool
}

// ValidateCron is the one entry point the provider's plan, a conforming host,
// and the conformance runner share. A nil error means every conforming host
// accepts the expression; a non-nil error names exactly which field is wrong.
func ValidateCron(expression string) error {
	_, err := ParseCron(expression)
	return err
}

// ParseCron parses one portable cron expression into its AST.
//
// It re-derives the structure rather than trusting PatternCron: the host runs
// it on a spec it may have received without a schema check, and a validator
// that assumes a prior check is a validator that fails open.
func ParseCron(expression string) (CronSchedule, error) {
	if expression == "" {
		return CronSchedule{}, errors.New("a cron expression is required")
	}
	fields := strings.Split(expression, " ")
	if len(fields) != CronFieldCount {
		return CronSchedule{}, fmt.Errorf(
			"a cron expression has exactly %d fields separated by single spaces: minute hour day-of-month month day-of-week; got %d",
			CronFieldCount, len(fields),
		)
	}
	values := make([][]int, CronFieldCount)
	for index, field := range fields {
		parsed, err := parseCronField(cronFields[index], field)
		if err != nil {
			return CronSchedule{}, err
		}
		values[index] = parsed
	}
	schedule := CronSchedule{
		Minute:     values[0],
		Hour:       values[1],
		DayOfMonth: values[2],
		Month:      values[3],
		DayOfWeek:  values[4],
	}
	schedule.dayOfMonthRestricted = len(values[2]) < cronFields[2].max-cronFields[2].min+1
	schedule.dayOfWeekRestricted = len(values[4]) < cronFields[4].max-cronFields[4].min+1
	return schedule, nil
}

// parseCronField expands one comma-separated term list into a sorted,
// de-duplicated value set.
func parseCronField(spec cronFieldSpec, field string) ([]int, error) {
	if field == "" {
		return nil, fmt.Errorf("the %s field is empty", spec.name)
	}
	seen := map[int]bool{}
	for _, term := range strings.Split(field, ",") {
		if err := expandCronTerm(spec, term, seen); err != nil {
			return nil, err
		}
	}
	out := make([]int, 0, len(seen))
	for value := range seen {
		out = append(out, value)
	}
	sort.Ints(out)
	return out, nil
}

// expandCronTerm expands one term into seen. The accepted forms are `*`,
// `*/step`, a literal, `low-high`, and `low-high/step`. A step on a bare
// literal (`5/10`) is deliberately NOT accepted: it means "from 5 to the field
// maximum" in some implementations and "exactly 5" in others, and a portable
// contract cannot carry a form whose meaning is contested.
func expandCronTerm(spec cronFieldSpec, term string, seen map[int]bool) error {
	if term == "" {
		return fmt.Errorf("the %s field carries an empty term", spec.name)
	}
	body, stepText, hasStep := strings.Cut(term, "/")
	step := 1
	if hasStep {
		parsed, err := parseCronNumber(spec, stepText, "step")
		if err != nil {
			return err
		}
		span := spec.max - spec.min
		if parsed < 1 || parsed > span {
			return fmt.Errorf(
				"the %s step %q is out of range; a step is between 1 and %d, the span of %d-%d",
				spec.name, stepText, span, spec.min, spec.max,
			)
		}
		step = parsed
	}
	low, high := spec.min, spec.max
	switch {
	case body == "*":
		// The whole domain, optionally stepped.
	case strings.Contains(body, "-"):
		lowText, highText, _ := strings.Cut(body, "-")
		parsedLow, err := parseCronValue(spec, lowText)
		if err != nil {
			return err
		}
		parsedHigh, err := parseCronValue(spec, highText)
		if err != nil {
			return err
		}
		if parsedLow > parsedHigh {
			return fmt.Errorf(
				"the %s range %q is inverted; a range runs from the lower value to the higher one",
				spec.name, body,
			)
		}
		low, high = parsedLow, parsedHigh
	default:
		if hasStep {
			return fmt.Errorf(
				"the %s term %q applies a step to a single value; a step applies to %q or to a range such as %q",
				spec.name, term, "*", fmt.Sprintf("%d-%d", spec.min, spec.max),
			)
		}
		parsed, err := parseCronValue(spec, body)
		if err != nil {
			return err
		}
		low, high = parsed, parsed
	}
	for value := low; value <= high; value += step {
		seen[value] = true
	}
	return nil
}

// parseCronValue parses one field value and holds it to the field's domain.
func parseCronValue(spec cronFieldSpec, text string) (int, error) {
	value, err := parseCronNumber(spec, text, "value")
	if err != nil {
		return 0, err
	}
	if value < spec.min || value > spec.max {
		return 0, fmt.Errorf(
			"the %s value %q is out of range; %s is %d-%d",
			spec.name, text, spec.name, spec.min, spec.max,
		)
	}
	return value, nil
}

// parseCronNumber accepts one or two decimal digits and nothing else, so a
// sign, a space, or a name is refused before any range question is asked.
func parseCronNumber(spec cronFieldSpec, text, role string) (int, error) {
	if len(text) == 0 || len(text) > 2 {
		return 0, fmt.Errorf("the %s %s %q is not one or two decimal digits", spec.name, role, text)
	}
	for _, digit := range text {
		if digit < '0' || digit > '9' {
			return 0, fmt.Errorf("the %s %s %q is not one or two decimal digits", spec.name, role, text)
		}
	}
	value, err := strconv.Atoi(text)
	if err != nil {
		return 0, fmt.Errorf("the %s %s %q is not one or two decimal digits", spec.name, role, text)
	}
	return value, nil
}

// Canonical renders the schedule's one canonical spelling: five fields, each
// `*` when it selects its whole domain, otherwise the ascending value set with
// every run of three or more consecutive values collapsed to a range.
//
// It is a comparison form, not a rewrite. A host stores the expression an
// author wrote, byte for byte, because the desired state is those bytes and a
// silently rewritten spec would make every plan drift. What the canonical form
// gives is the answer to "are these two expressions the same schedule", which
// `*/5 * * * *` and `0,5,10,15,20,25,30,35,40,45,50,55 * * * *` need in order
// to be the same thing.
func (schedule CronSchedule) Canonical() string {
	parts := make([]string, 0, CronFieldCount)
	for index, values := range [][]int{
		schedule.Minute, schedule.Hour, schedule.DayOfMonth, schedule.Month, schedule.DayOfWeek,
	} {
		parts = append(parts, canonicalCronField(cronFields[index], values))
	}
	return strings.Join(parts, " ")
}

func canonicalCronField(spec cronFieldSpec, values []int) string {
	if len(values) == spec.max-spec.min+1 {
		return "*"
	}
	var terms []string
	for start := 0; start < len(values); {
		end := start
		for end+1 < len(values) && values[end+1] == values[end]+1 {
			end++
		}
		switch {
		case end-start >= 2:
			terms = append(terms, strconv.Itoa(values[start])+"-"+strconv.Itoa(values[end]))
		default:
			for index := start; index <= end; index++ {
				terms = append(terms, strconv.Itoa(values[index]))
			}
		}
		start = end + 1
	}
	return strings.Join(terms, ",")
}

// Matches reports whether one instant is a match of this schedule. The instant
// is evaluated in UTC whatever zone it carries, because the contract states UTC
// and nothing else.
//
// The day rule is the one place cron is not a plain conjunction, and it is
// stated rather than inherited: when BOTH day-of-month and day-of-week are
// restricted, an instant matches if EITHER of them matches. When only one is
// restricted, only that one constrains the day. When neither is, every day
// matches. That is the long-standing behavior of the widely deployed
// implementations, and choosing the other reading would silently change what
// `0 3 1 * 1` means for every author who has ever written a crontab.
func (schedule CronSchedule) Matches(instant time.Time) bool {
	utc := instant.UTC()
	if !containsInt(schedule.Minute, utc.Minute()) ||
		!containsInt(schedule.Hour, utc.Hour()) ||
		!containsInt(schedule.Month, int(utc.Month())) {
		return false
	}
	dayOfMonth := containsInt(schedule.DayOfMonth, utc.Day())
	dayOfWeek := containsInt(schedule.DayOfWeek, int(utc.Weekday()))
	switch {
	case schedule.dayOfMonthRestricted && schedule.dayOfWeekRestricted:
		return dayOfMonth || dayOfWeek
	case schedule.dayOfMonthRestricted:
		return dayOfMonth
	case schedule.dayOfWeekRestricted:
		return dayOfWeek
	default:
		return true
	}
}

func containsInt(values []int, want int) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
