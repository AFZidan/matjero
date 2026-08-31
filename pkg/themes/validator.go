package themes

import (
	"fmt"
	"regexp"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// unsafeContentPatterns rejects configuration strings that could introduce
// executable content. Seller theme customization is data/configuration only; it
// must never be able to inject scripts, javascript: URLs, inline event handlers,
// embedded objects/iframes, or CSS expressions.
var unsafeContentPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)<script`),
	regexp.MustCompile(`(?i)javascript:`),
	regexp.MustCompile(`(?i)\bon\w+\s*=`),
	regexp.MustCompile(`(?i)<iframe`),
	regexp.MustCompile(`(?i)<object`),
	regexp.MustCompile(`(?i)<embed`),
	regexp.MustCompile(`(?i)<style`),
	regexp.MustCompile(`(?i)expression\s*\(`),
}

// ValidateConfiguration validates config against the theme version's JSON Schema.
// The schema is compiled and the instance is checked; a mismatch returns
// ErrSchemaMismatch. An invalid schema definition returns ErrInvalidInput.
func ValidateConfiguration(schema, config map[string]any) error {
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource("schema.json", schema); err != nil {
		return fmt.Errorf("%w: invalid schema: %v", ErrInvalidInput, err)
	}
	sch, err := compiler.Compile("schema.json")
	if err != nil {
		return fmt.Errorf("%w: invalid schema: %v", ErrInvalidInput, err)
	}
	if err := sch.Validate(config); err != nil {
		return fmt.Errorf("%w: %v", ErrSchemaMismatch, err)
	}
	return nil
}

// RejectUnsafeContent recursively scans the configuration for strings that could
// introduce executable content. It returns ErrUnsafeContent if any are found.
func RejectUnsafeContent(value any) error {
	return rejectUnsafe(value)
}

func rejectUnsafe(value any) error {
	switch v := value.(type) {
	case string:
		for _, re := range unsafeContentPatterns {
			if re.MatchString(v) {
				return fmt.Errorf("%w: prohibited pattern %q", ErrUnsafeContent, re.String())
			}
		}
	case map[string]any:
		for _, item := range v {
			if err := rejectUnsafe(item); err != nil {
				return err
			}
		}
	case []any:
		for _, item := range v {
			if err := rejectUnsafe(item); err != nil {
				return err
			}
		}
	}
	return nil
}
