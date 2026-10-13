//go:build !linux && !darwin && !freebsd && !openbsd && !netbsd

package intentv2

// A platform without an owner-checked, no-follow file reader must not adopt
// Runner grants by pathname.
func readPrivateDescriptor(string, int) ([]byte, error) {
	return nil, ErrStoreUnavailable
}

func clearDescriptorBytes(data []byte) {
	for i := range data {
		data[i] = 0
	}
}
