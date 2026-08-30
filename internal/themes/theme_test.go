package themes

import (
	"strings"
	"testing"
)

func TestValidThemeTypeAndStatus(t *testing.T) {
	for _, typ := range []string{ThemeTypeFree, ThemeTypePremium} {
		if !ValidThemeType(typ) {
			t.Errorf("expected theme type %q to be valid", typ)
		}
	}
	if ValidThemeType("bogus") {
		t.Error("expected bogus theme type to be invalid")
	}
	for _, st := range []string{ThemeStatusDraft, ThemeStatusActive, ThemeStatusDeprecated, ThemeStatusDisabled} {
		if !ValidThemeStatus(st) {
			t.Errorf("expected theme status %q to be valid", st)
		}
	}
	if ValidThemeStatus("bogus") {
		t.Error("expected bogus theme status to be invalid")
	}
	for _, st := range []string{ThemeVersionStatusDraft, ThemeVersionStatusPublished, ThemeVersionStatusDeprecated} {
		if !ValidThemeVersionStatus(st) {
			t.Errorf("expected theme version status %q to be valid", st)
		}
	}
	for _, st := range []string{ThemeInstallationStatusActive, ThemeInstallationStatusInactive} {
		if !ValidThemeInstallationStatus(st) {
			t.Errorf("expected theme installation status %q to be valid", st)
		}
	}
}

func TestValidateConfigurationAcceptsValid(t *testing.T) {
	schema := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"title": map[string]any{"type": "string"},
		},
	}
	if err := ValidateConfiguration(schema, map[string]any{"title": "Hello"}); err != nil {
		t.Fatalf("expected valid config to pass, got %v", err)
	}
}

func TestValidateConfigurationRejectsInvalid(t *testing.T) {
	schema := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"title": map[string]any{"type": "string"},
		},
	}
	// additionalProperties:false rejects unknown keys
	if err := ValidateConfiguration(schema, map[string]any{"title": "Hi", "evil": 1}); err == nil {
		t.Fatal("expected unknown property to be rejected")
	}
	// wrong type
	if err := ValidateConfiguration(schema, map[string]any{"title": 123}); err == nil {
		t.Fatal("expected wrong type to be rejected")
	}
}

func TestRejectUnsafeContent(t *testing.T) {
	cases := []string{
		"<script>alert(1)</script>",
		"javascript:alert(1)",
		"onerror=alert(1)",
		"<iframe src='x'></iframe>",
		"<object data='x'></object>",
		"<style>body{}</style>",
		"expression(alert(1))",
	}
	for _, c := range cases {
		cfg := map[string]any{"hero": map[string]any{"title": c}}
		if err := RejectUnsafeContent(cfg); err == nil {
			t.Errorf("expected unsafe content %q to be rejected", c)
		}
	}
}

func TestRejectUnsafeContentAllowsSafe(t *testing.T) {
	safe := map[string]any{
		"hero": map[string]any{
			"title":    "Summer Sale",
			"cta_url":  "https://example.com/sale",
			"image_url": "https://cdn.example.com/hero.png",
		},
		"colors": map[string]any{"primary": "#0f766e"},
	}
	if err := RejectUnsafeContent(safe); err != nil {
		t.Fatalf("expected safe config to pass, got %v", err)
	}
}

func TestDefaultConfigValidatesAgainstSchema(t *testing.T) {
	if err := ValidateConfiguration(DefaultConfigurationSchema, DefaultConfiguration); err != nil {
		t.Fatalf("built-in default theme config must validate against its schema: %v", err)
	}
	if err := RejectUnsafeContent(DefaultConfiguration); err != nil {
		t.Fatalf("built-in default theme config must be safe: %v", err)
	}
}

func TestUnsafeContentDetectionIsCaseInsensitive(t *testing.T) {
	if err := RejectUnsafeContent(map[string]any{"x": strings.ToUpper("<SCRIPT>")}); err == nil {
		t.Fatal("expected case-insensitive <SCRIPT> to be rejected")
	}
}
