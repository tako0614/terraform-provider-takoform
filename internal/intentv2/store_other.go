//go:build !linux && !darwin && !freebsd && !openbsd && !netbsd

package intentv2

import "context"

// Non-POSIX platforms remain buildable, but this prerequisite refuses to open
// or initialize stores until an equivalent secure lock/filesystem backend is
// implemented and reviewed for that platform.
type secureDirectory struct{}

func createSecureDirectory(string) (*secureDirectory, error)  { return nil, ErrStoreUnavailable }
func openSecureDirectory(string) (*secureDirectory, error)    { return nil, ErrStoreUnavailable }
func (*secureDirectory) close() error                         { return ErrStoreUnavailable }
func (*secureDirectory) verifyPathIdentity() error            { return ErrStoreUnavailable }
func (*secureDirectory) validateStoreEntries() error          { return ErrStoreUnavailable }
func (*secureDirectory) initializeFiles([]byte, []byte) error { return ErrStoreUnavailable }
func (*secureDirectory) readFile(string, int) ([]byte, error) { return nil, ErrStoreUnavailable }
func (*secureDirectory) replaceFile(string, []byte) error     { return ErrStoreUnavailable }
func (*secureDirectory) withLock(context.Context, func() error) error {
	return ErrStoreUnavailable
}
