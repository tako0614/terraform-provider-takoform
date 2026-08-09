package workerauthoring

import (
	"context"
	"errors"
	"fmt"
	"reflect"
)

// MatrixReport is the evidence both supported CLIs produced.
type MatrixReport struct {
	Format           string   `json:"format"`
	PublicationReady bool     `json:"publicationReady"`
	Reports          []Report `json:"reports"`
}

// MatrixFormat identifies the two-CLI evidence shape.
const MatrixFormat = "takoform.worker-authoring-conformance-matrix@v1"

// RunMatrix drives every scenario under both supported CLIs.
func RunMatrix(ctx context.Context, repoRoot, openTofuPath, terraformPath string) (MatrixReport, error) {
	matrix := MatrixReport{Format: MatrixFormat}
	for _, cli := range []string{openTofuPath, terraformPath} {
		report, err := Run(ctx, repoRoot, cli)
		if err != nil {
			return MatrixReport{}, err
		}
		matrix.Reports = append(matrix.Reports, report)
	}
	return matrix, nil
}

// ValidateMatrix holds the evidence to what the authoring surface promises,
// and to the two CLIs agreeing about it.
func ValidateMatrix(matrix MatrixReport) error {
	if matrix.Format != MatrixFormat || matrix.PublicationReady {
		return errors.New("worker authoring matrix identity drifted")
	}
	if len(matrix.Reports) != 2 {
		return fmt.Errorf("worker authoring matrix carries %d CLI reports, want 2", len(matrix.Reports))
	}
	products := map[string]bool{}
	for _, report := range matrix.Reports {
		if err := Validate(report); err != nil {
			return err
		}
		products[report.CLI.Product] = true
	}
	if !products["OpenTofu"] || !products["Terraform"] {
		return errors.New("worker authoring matrix must cover OpenTofu and Terraform")
	}
	first, second := matrix.Reports[0], matrix.Reports[1]
	if !reflect.DeepEqual(first.SameNameDeadlock, second.SameNameDeadlock) {
		return errors.New("OpenTofu and Terraform observed different host refusals")
	}
	if !reflect.DeepEqual(first.PlanRefusal, second.PlanRefusal) ||
		!reflect.DeepEqual(first.HostSupport, second.HostSupport) {
		return errors.New("OpenTofu and Terraform observed different plan refusals")
	}
	if !reflect.DeepEqual(first.RollForward.Mutations, second.RollForward.Mutations) ||
		!reflect.DeepEqual(first.ModuleDeploy.Mutations, second.ModuleDeploy.Mutations) {
		return errors.New("OpenTofu and Terraform drove different roll-forward sequences")
	}
	if !reflect.DeepEqual(first.Configurations, second.Configurations) {
		return errors.New("OpenTofu and Terraform validated different configurations")
	}
	return nil
}

// Validate holds one CLI's evidence to the claims this lane makes.
func Validate(report Report) error {
	if report.Format != ReportFormat || report.PublicationReady {
		return errors.New("worker authoring report identity drifted")
	}
	if report.CLI.Product == "" || report.CLI.Version == "" {
		return errors.New("worker authoring report names no CLI")
	}
	// The deadlock is the finding the derived names exist for, so it is asserted
	// by its exact codes rather than by "something failed".
	if report.SameNameDeadlock.DestroyFirstCode != "dependency_in_use" ||
		report.SameNameDeadlock.DestroyFirstHTTP != 409 {
		return fmt.Errorf("destroy-first refusal was %s (%d), want dependency_in_use (409)",
			report.SameNameDeadlock.DestroyFirstCode, report.SameNameDeadlock.DestroyFirstHTTP)
	}
	if report.SameNameDeadlock.CreateFirstCode != "invalid_argument" ||
		report.SameNameDeadlock.CreateFirstHTTP != 400 {
		return fmt.Errorf("create-first refusal was %s (%d), want invalid_argument (400)",
			report.SameNameDeadlock.CreateFirstCode, report.SameNameDeadlock.CreateFirstHTTP)
	}
	if report.PlanRefusal.Code != "takoform.provider/immutable-revision-same-name" {
		return errors.New("the same-name replacement was not refused at plan")
	}
	if report.HostSupport.Code != "takoform.provider/host-does-not-support-form" {
		return errors.New("an unsupported Form was not refused at plan")
	}
	for name, sequence := range map[string]SequenceEvidence{
		"raw resources":     report.RollForward,
		"worker-app module": report.ModuleDeploy,
	} {
		if err := assertRollForwardOrder(sequence.Mutations); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		if sequence.ReadySamples == 0 || sequence.NotReadySamples != 0 {
			return fmt.Errorf("%s: the worker was observed unserved during the roll-forward (%d of %d)",
				name, sequence.NotReadySamples, sequence.ReadySamples+sequence.NotReadySamples)
		}
	}
	if !report.ModuleDeploy.EndpointURLStable {
		return errors.New("the host-assigned endpoint address did not survive the code change")
	}
	if len(report.Configurations) == 0 {
		return errors.New("no configuration was validated")
	}
	return nil
}
