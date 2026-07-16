package formpackage

import (
	"fmt"
	"strings"
	"unicode"
)

var forbiddenFieldFragments = []string{
	"credential",
	"secret",
	"password",
	"privatekey",
	"apikey",
	"token",
	"account",
	"accesstoken",
	"refreshtoken",
	"accountid",
	"target",
	"poolid",
	"capacity",
	"backendmanager",
	"providerconfig",
	"price",
	"pricing",
	"sku",
	"billing",
	"quota",
	"servicelevelagreement",
	"slapolicy",
	"supportpolicy",
	"operator",
	"executable",
	"command",
	"script",
	"sourcecode",
	"validationcode",
	"adaptercode",
	"runtimecode",
	"bytecode",
	"webassembly",
	"wasm",
	"plugin",
}

var forbiddenExactFields = map[string]struct{}{
	"binary": {},
	"code":   {},
	"exec":   {},
}

func rejectForbiddenContent(value any, location string) error {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			normalized := normalizeFieldName(key)
			if _, forbidden := forbiddenExactFields[normalized]; forbidden {
				return fmt.Errorf("forbidden field %q at %s", key, location)
			}
			for _, forbidden := range forbiddenFieldFragments {
				if strings.Contains(normalized, forbidden) {
					return fmt.Errorf("forbidden field %q at %s", key, location)
				}
			}
			if err := rejectForbiddenContent(child, location+"."+key); err != nil {
				return err
			}
		}
	case []any:
		for index, child := range typed {
			if err := rejectForbiddenContent(child, fmt.Sprintf("%s[%d]", location, index)); err != nil {
				return err
			}
		}
	}
	return nil
}

func normalizeFieldName(value string) string {
	var normalized strings.Builder
	for _, character := range value {
		if unicode.IsLetter(character) || unicode.IsDigit(character) {
			normalized.WriteRune(unicode.ToLower(character))
		}
	}
	return normalized.String()
}
