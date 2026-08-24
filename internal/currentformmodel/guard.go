package currentformmodel

import (
	"fmt"
	"strings"
)

// openTokenFieldNames are wire names that carried open semantic tokens in the
// superseded category-shaped lanes. Decision 0008 prohibits them: a value a
// host may interpret freely turns one desired document into different systems
// on different hosts. A closed enum may reuse a name; a free string may not.
var openTokenFieldNames = map[string]struct{}{
	"runtime":           {},
	"engine":            {},
	"persistence":       {},
	"consistency":       {},
	"ordering":          {},
	"deliveryguarantee": {},
	"projection":        {},
	"permission":        {},
	"portability":       {},
	"bindingtype":       {},
}

// ValidateNoOpenTokens fails the catalog when any free-string field reuses an
// open-token name from the superseded lanes, at any nesting depth.
func ValidateNoOpenTokens(forms []Form) error {
	for _, form := range forms {
		if err := validateNoOpenTokenFields(form.Kind, form.Fields); err != nil {
			return err
		}
	}
	return nil
}

func validateNoOpenTokenFields(kind string, fields []Field) error {
	for _, field := range fields {
		if field.Kind == KindString {
			if _, banned := openTokenFieldNames[strings.ToLower(field.Wire)]; banned {
				return fmt.Errorf("form %s field %s is an open-token name; semantics live in separate Forms, closed enums, or typed Bindings (decision 0008)", kind, field.Wire)
			}
		}
		if err := validateNoOpenTokenFields(kind, field.Fields); err != nil {
			return err
		}
		for _, variant := range field.Variants {
			if err := validateNoOpenTokenFields(kind, variant.Fields); err != nil {
				return err
			}
		}
	}
	return nil
}
