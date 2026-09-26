package workerauthoring

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestHasExternalProviderRequirement(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name   string
		output string
		want   bool
	}{
		{
			name:   "repository provider only",
			output: "└── provider[registry.terraform.io/tako0614/takoform] ~> 4.0",
		},
		{
			name: "native AWS peer",
			output: "├── provider[registry.terraform.io/tako0614/takoform] ~> 4.0\n" +
				"└── provider[registry.terraform.io/hashicorp/aws] ~> 6.0",
			want: true,
		},
		{
			name:   "built in provider",
			output: "└── provider[terraform.io/builtin/terraform]",
		},
		{
			name:   "malformed provider output fails closed",
			output: "└── provider[registry.terraform.io/hashicorp/aws",
			want:   true,
		},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := hasExternalProviderRequirement(test.output); got != test.want {
				t.Fatalf("hasExternalProviderRequirement() = %t, want %t", got, test.want)
			}
		})
	}
}

func TestInspectProviderRequirementsDoesNotRequireLockedPeerPackage(t *testing.T) {
	tofu, err := exec.LookPath("tofu")
	if err != nil {
		t.Skip("OpenTofu is unavailable")
	}
	workingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	repoRoot, err := RepoRoot(workingDirectory)
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(repoRoot, "examples", "getting-started")
	directory := filepath.Join(t.TempDir(), "getting-started")
	if err := copyTree(source, directory); err != nil {
		t.Fatal(err)
	}
	lockPath := filepath.Join(directory, ".terraform.lock.hcl")
	lockBefore, err := os.ReadFile(lockPath)
	if err != nil {
		t.Fatal(err)
	}

	env := append(sanitizedEnvironment(), "TF_PLUGIN_CACHE_DIR=")
	lockedOutput, lockedErr := runCommand(context.Background(), repoRoot, env, tofu,
		"-chdir="+directory, "providers", "-no-color")
	if lockedErr == nil || !strings.Contains(lockedOutput, "hashicorp/random") {
		t.Fatalf("test fixture no longer reproduces a locked peer without its installed package: err=%v output=%s", lockedErr, lockedOutput)
	}

	output, err := inspectProviderRequirements(context.Background(), repoRoot, tofu, env, directory)
	if err != nil {
		t.Fatalf("inspect locked peer requirements: %v\n%s", err, output)
	}
	if !hasExternalProviderRequirement(output) || !strings.Contains(output, "hashicorp/random") {
		t.Fatalf("peer provider was not classified from the configuration: %s", output)
	}
	lockAfter, err := os.ReadFile(lockPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(lockAfter) != string(lockBefore) {
		t.Fatal("provider requirements inspection changed the scratch lockfile")
	}
}
