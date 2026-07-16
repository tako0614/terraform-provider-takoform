package formpackage

import (
	"fmt"
	"strings"
	"unicode"
)

// forbiddenNormalizedFields is intentionally exact. Substring matching is
// unsafe here: for example, the legitimate JSON Schema keyword "description"
// contains "script". Variants with camel/snake/kebab boundaries are covered
// by forbiddenFieldTokens below.
var forbiddenNormalizedFields = stringSet(
	// Credentials, authentication, and secret-bearing connection material.
	"credential", "credentials", "credentialid", "credentialids", "credentialref", "credentialrefs", "credentialname", "credentialvalue",
	"secret", "secrets", "secretid", "secretids", "secretref", "secretrefs", "secretname", "secretvalue",
	"password", "passwords", "passphrase", "privatekey", "privatekeyid", "privatekeyref", "apikey", "apikeyid", "apikeyref",
	"apikeyvalue", "privatekeypem", "sshprivatekey",
	"token", "tokens", "tokenid", "tokenref", "accesstoken", "refreshtoken", "idtoken", "bearertoken",
	"authorization", "authorizationheader", "authheader", "bearer", "oauth", "oauthclient", "oauthclientid", "oauthclientsecret", "oidcclientsecret",
	"sessioncookie", "sessiontoken", "cookie", "cookies", "connectionstring", "signingkey", "sshkey",

	// Operator, backend, account, placement, and live capacity authority.
	"operator", "operators", "operatorid", "operatorpolicy", "account", "accounts", "accountid",
	"target", "targets", "targetid", "targetpool", "targetpoolid", "poolid",
	"capacity", "activecapacity", "regioncapacity", "backendmanager", "managerid", "manageridentifier",
	"provider", "providerid", "providername", "providerconfig", "backend", "backendid",
	"implementationid", "selectedimplementation", "region", "regions", "regionid", "zone", "zones", "zoneid", "placement",

	// Commercial and service-operation authority.
	"price", "prices", "pricing", "priceid", "unitprice", "monthlyprice", "sku", "skus",
	"billing", "billingplan", "billingaccount", "invoice", "invoices", "invoiceid",
	"payment", "payments", "paymentid", "paymentmethod", "paymentmethods",
	"currency", "currencies", "currencycode", "tax", "taxes", "taxcode", "taxrate",
	"quota", "quotas", "sla", "slapolicy", "servicelevelagreement", "supportpolicy",
	"serviceoffering", "serviceofferings", "serviceofferingid", "subscription", "subscriptions", "entitlement", "entitlements",

	// Executable or host-extension material.
	"binary", "code", "exec", "executable", "command", "commands", "script", "scripts",
	"sourcecode", "validationcode", "adapter", "adaptercode", "runtimecode", "bytecode",
	"webassembly", "wasm", "plugin", "plugins",
)

var forbiddenFieldTokens = stringSet(
	"credential", "secret", "password", "passphrase", "token", "authorization", "bearer", "oauth", "cookie",
	"operator", "account", "target", "capacity", "provider", "backend", "implementation", "region", "zone", "placement",
	"price", "pricing", "sku", "billing", "invoice", "payment", "currency", "tax", "quota", "sla", "subscription", "entitlement",
	"binary", "code", "exec", "executable", "command", "script", "bytecode", "wasm", "plugin",
)

// Some sensitive concepts are compounds whose individual words are useful in
// portable schemas. Match reviewed boundary-delimited token sequences instead
// of unsafe substrings: "apiKeyValue" is forbidden, while "description" and
// prose containing "API key" are unaffected because only field names enter
// this function.
var forbiddenFieldTokenSequences = [][]string{
	{"api", "key"},
	{"private", "key"},
	{"ssh", "key"},
	{"signing", "key"},
	{"service", "offering"},
	{"backend", "manager"},
	{"manager", "id"},
	{"manager", "identifier"},
}

var forbiddenNormalizedCompoundBases = []string{
	"apikey",
	"privatekey",
	"sshprivatekey",
	"signingkey",
	"serviceoffering",
	"backendmanager",
	"managerid",
	"manageridentifier",
}

var forbiddenCompoundQualifiers = stringSet(
	"id", "ids", "identifier", "identifiers", "ref", "refs", "name", "names",
	"value", "values", "pem", "material", "fingerprint", "header", "path", "file",
	"config", "configuration", "label", "labels",
)

func rejectForbiddenContent(value any, location string) error {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if isForbiddenFieldName(key) {
				return fmt.Errorf("forbidden field %q at %s", key, location)
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

func isForbiddenFieldName(value string) bool {
	normalized := normalizeFieldName(value)
	if _, forbidden := forbiddenNormalizedFields[normalized]; forbidden {
		return true
	}
	for _, base := range forbiddenNormalizedCompoundBases {
		if !strings.HasPrefix(normalized, base) {
			continue
		}
		if _, forbidden := forbiddenCompoundQualifiers[strings.TrimPrefix(normalized, base)]; forbidden {
			return true
		}
	}
	tokens := splitFieldNameTokens(value)
	for _, token := range tokens {
		if _, forbidden := forbiddenFieldTokens[token]; forbidden {
			return true
		}
	}
	for _, sequence := range forbiddenFieldTokenSequences {
		if containsTokenSequence(tokens, sequence) {
			return true
		}
	}
	return false
}

func containsTokenSequence(tokens, sequence []string) bool {
	if len(sequence) == 0 || len(tokens) < len(sequence) {
		return false
	}
	for start := 0; start <= len(tokens)-len(sequence); start++ {
		matched := true
		for offset := range sequence {
			if tokens[start+offset] != sequence[offset] {
				matched = false
				break
			}
		}
		if matched {
			return true
		}
	}
	return false
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

func splitFieldNameTokens(value string) []string {
	runes := []rune(value)
	tokens := []string{}
	var current strings.Builder
	flush := func() {
		if current.Len() == 0 {
			return
		}
		tokens = append(tokens, strings.ToLower(current.String()))
		current.Reset()
	}
	for index, character := range runes {
		if !unicode.IsLetter(character) && !unicode.IsDigit(character) {
			flush()
			continue
		}
		if current.Len() > 0 {
			previous := runes[index-1]
			nextIsLower := index+1 < len(runes) && unicode.IsLower(runes[index+1])
			caseBoundary := unicode.IsUpper(character) && (unicode.IsLower(previous) || unicode.IsDigit(previous) || (unicode.IsUpper(previous) && nextIsLower))
			digitBoundary := unicode.IsDigit(character) != unicode.IsDigit(previous)
			if caseBoundary || digitBoundary {
				flush()
			}
		}
		current.WriteRune(character)
	}
	flush()
	return tokens
}

func stringSet(values ...string) map[string]struct{} {
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		result[value] = struct{}{}
	}
	return result
}
