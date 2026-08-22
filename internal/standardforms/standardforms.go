// Package standardforms renders and verifies the catalog-derived public
// surfaces of the Edge Platform Family. The central-epoch machinery that used
// to share this package — lifecycle authority, release plans, legacy package
// and admission verification — was withdrawn with the pre-Beta generations it
// served (decision 0042); the bytes remain in this repository's history.
package standardforms

// Verify fails closed when any generated public surface differs from the
// canonical catalog rendering, is missing, or is joined by an undeclared file.
// It never writes the worktree.
func Verify(root string) error {
	return VerifyPublishedSurfaces(root)
}
