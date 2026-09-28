//go:build !linux && !darwin && !freebsd && !openbsd && !netbsd

package provider

import "errors"

var errBearerTokenFile = errors.New("takoform: secure bearer token file access unsupported on this platform")

// There is no portable no-follow, owner-checked file open on this platform.
// The static token configuration remains available.
func readBearerTokenFile(string) (string, error) { return "", errBearerTokenFile }
