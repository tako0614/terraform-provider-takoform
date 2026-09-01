package workerauthoring

import "testing"

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
