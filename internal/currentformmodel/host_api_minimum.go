package currentformmodel

import (
	"fmt"
	"regexp"
	"strconv"
)

const (
	firstHostAPILane  = "forms.takoform.com/v1beta1"
	stableHostAPILane = "forms.takoform.com/v1"
)

// minimumRequiredHostAPI derives the earliest substrate lane needed by the
// mechanisms this Definition actually uses. It deliberately does not map Form
// kind names to lanes: families and Host releases move independently, and a
// Form earns a higher minimum only by declaring a mechanism unavailable in an
// earlier Host API.
func (f Form) minimumRequiredHostAPI() string {
	if len(f.StructuralConstraints) > 0 || len(f.ResolvedUIDConstraints) > 0 || fieldsRequireStableHostAPI(f.Fields) {
		return stableHostAPILane
	}
	return firstHostAPILane
}

func fieldsRequireStableHostAPI(fields []Field) bool {
	for _, field := range fields {
		// The retained TargetKind spelling may carry RequiredEntrypoint on a
		// v1beta1 Form whose lane hard-coded the equivalent Form-specific gate.
		// Only a new aggregate ResourceTarget asks the Form-neutral host to read
		// the annotation as a mechanism, so published v1beta1 contracts retain
		// their declared lower bound unchanged.
		if field.Kind == KindTaggedObject ||
			field.Kind == KindExternalServiceList ||
			(field.RequiredEntrypoint != "" && field.ResourceTarget != nil) {
			return true
		}
		if fieldsRequireStableHostAPI(field.Fields) {
			return true
		}
		for _, variant := range field.Variants {
			if fieldsRequireStableHostAPI(variant.Fields) {
				return true
			}
		}
	}
	return false
}

type hostAPILane struct {
	major int
	phase int
	index int
}

var hostAPILanePartsPattern = regexp.MustCompile(
	`^forms\.takoform\.com/v([0-9]+)(?:(alpha|beta)([0-9]+))?$`,
)

func parseHostAPILane(identity string) (hostAPILane, error) {
	parts := hostAPILanePartsPattern.FindStringSubmatch(identity)
	if parts == nil {
		return hostAPILane{}, fmt.Errorf("%q is not a Host API lane identity", identity)
	}
	major, err := strconv.Atoi(parts[1])
	if err != nil {
		return hostAPILane{}, fmt.Errorf("parse Host API major in %q: %w", identity, err)
	}
	lane := hostAPILane{major: major, phase: 2}
	if parts[2] == "" {
		return lane, nil
	}
	lane.phase = 0
	if parts[2] == "beta" {
		lane.phase = 1
	}
	lane.index, err = strconv.Atoi(parts[3])
	if err != nil {
		return hostAPILane{}, fmt.Errorf("parse Host API prerelease in %q: %w", identity, err)
	}
	return lane, nil
}

func hostAPILaneAtLeast(declared, minimum string) (bool, error) {
	got, err := parseHostAPILane(declared)
	if err != nil {
		return false, err
	}
	want, err := parseHostAPILane(minimum)
	if err != nil {
		return false, err
	}
	if got.major != want.major {
		return got.major > want.major, nil
	}
	if got.phase != want.phase {
		return got.phase > want.phase, nil
	}
	return got.index >= want.index, nil
}
