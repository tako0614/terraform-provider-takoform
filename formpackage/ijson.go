package formpackage

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// DecodeStrictIJSON validates the complete raw document as UTF-8 I-JSON before
// typed decoding. This preserves duplicate-member, Unicode, number, depth, and
// trailing-value failures that encoding/json would otherwise collapse or
// normalize before DisallowUnknownFields can inspect the target shape.
func DecodeStrictIJSON(input []byte, target any) error {
	if _, err := Canonicalize(input); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(input))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("typed I-JSON decode: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("typed I-JSON decode: document contains multiple JSON values")
		}
		return fmt.Errorf("typed I-JSON decode trailing value: %w", err)
	}
	return nil
}
