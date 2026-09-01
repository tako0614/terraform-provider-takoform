//go:build !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd

package provider

import "errors"

func secureReadV3RuntimeInputsFile(string) ([]byte, error) {
	return nil, errors.New("secure runtime-input file opening is unavailable on this operating system")
}
