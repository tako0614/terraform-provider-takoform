package standardforms

import (
	"path/filepath"
	"testing"
)

func TestCommittedStableSetVerifies(t *testing.T) {
	t.Parallel()
	if err := Verify(filepath.Join("..", "..")); err != nil {
		t.Fatal(err)
	}
}
