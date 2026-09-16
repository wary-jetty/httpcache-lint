package main

import (
	"reflect"
	"testing"
)

func TestSplitCacheControl(t *testing.T) {
	tests := []struct {
		raw  string
		want []string
	}{
		{"max-age=60", []string{"max-age=60"}},
		{"max-age=60, no-cache", []string{"max-age=60", " no-cache"}},
		{`no-cache="set-cookie,x-foo", max-age=60`, []string{`no-cache="set-cookie,x-foo"`, " max-age=60"}},
		{"max-age=60,,no-cache", []string{"max-age=60", "", "no-cache"}},
		{"", []string{""}},
	}
	for _, tt := range tests {
		got := splitCacheControl(tt.raw)
		if !reflect.DeepEqual(got, tt.want) {
			t.Errorf("splitCacheControl(%q) = %#v, want %#v", tt.raw, got, tt.want)
		}
	}
}

func TestParseDirective(t *testing.T) {
	tests := []struct {
		part string
		want Directive
	}{
		{"no-store", Directive{Name: "no-store", Value: "", HasValue: false}},
		{"Max-Age=60", Directive{Name: "max-age", Value: "60", HasValue: true}},
		{`no-cache="set-cookie"`, Directive{Name: "no-cache", Value: `"set-cookie"`, HasValue: true}},
		{"  max-age = 60  ", Directive{Name: "max-age", Value: "60", HasValue: true}},
	}
	for _, tt := range tests {
		got := parseDirective(tt.part)
		if got != tt.want {
			t.Errorf("parseDirective(%q) = %#v, want %#v", tt.part, got, tt.want)
		}
	}
}

func TestParseCacheControlUnrecognizedDirective(t *testing.T) {
	_, diags := ParseCacheControl("frobnicate")
	if !hasDiagWithMessage(diags, `unrecognized directive "frobnicate"`) {
		t.Errorf("expected unrecognized directive diagnostic, got %#v", diags)
	}
}

func TestParseCacheControlRepeatedDirective(t *testing.T) {
	_, diags := ParseCacheControl("max-age=60, max-age=120")
	if !hasDiagWithMessage(diags, `directive "max-age" repeated`) {
		t.Errorf("expected repeated directive diagnostic, got %#v", diags)
	}
}

func TestParseCacheControlDeltaSeconds(t *testing.T) {
	tests := []struct {
		raw     string
		wantErr bool
		wantMsg string
	}{
		{"max-age=60", false, ""},
		{"max-age", true, `directive "max-age" requires a delta-seconds value`},
		{"max-age=abc", true, `directive "max-age" value "abc" is not an integer number of seconds`},
		{"max-age=-5", true, `directive "max-age" must not be negative`},
	}
	for _, tt := range tests {
		_, diags := ParseCacheControl(tt.raw)
		gotErr := hasDiagWithSeverity(diags, SevError)
		if gotErr != tt.wantErr {
			t.Errorf("ParseCacheControl(%q) error presence = %v, want %v (%#v)", tt.raw, gotErr, tt.wantErr, diags)
		}
		if tt.wantMsg != "" && !hasDiagWithMessage(diags, tt.wantMsg) {
			t.Errorf("ParseCacheControl(%q): expected message %q, got %#v", tt.raw, tt.wantMsg, diags)
		}
	}
}

func TestParseCacheControlFlagDirectiveWithValue(t *testing.T) {
	_, diags := ParseCacheControl("no-store=yes")
	if !hasDiagWithMessage(diags, `directive "no-store" does not take a value, got "yes"`) {
		t.Errorf("expected no-store value diagnostic, got %#v", diags)
	}
}

func TestParseCacheControlOptionalFieldList(t *testing.T) {
	_, diags := ParseCacheControl(`no-cache="set-cookie, x-foo"`)
	if hasDiagWithSeverity(diags, SevError) || hasDiagWithSeverity(diags, SevWarning) {
		t.Errorf("well-formed field list should not raise diagnostics, got %#v", diags)
	}

	_, diags = ParseCacheControl("no-cache=set-cookie")
	if !hasDiagWithMessage(diags, `directive "no-cache" value should be a quoted, comma-separated field list, got "set-cookie"`) {
		t.Errorf("expected field list diagnostic, got %#v", diags)
	}
}

func TestCrossCheckPublicPrivateExclusive(t *testing.T) {
	directives, _ := ParseCacheControl("public, private")
	diags := crossCheckDirectives(directives)
	if !hasDiagWithMessage(diags, `"public" and "private" are mutually exclusive`) {
		t.Errorf("expected public/private conflict, got %#v", diags)
	}
}

func TestCrossCheckNoStoreMaxAgePointless(t *testing.T) {
	directives, _ := ParseCacheControl("no-store, max-age=60")
	diags := crossCheckDirectives(directives)
	if !hasDiagWithMessage(diags, `"no-store" makes "max-age=60" pointless`) {
		t.Errorf("expected no-store/max-age diagnostic, got %#v", diags)
	}
}

func TestCrossCheckImmutableMaxAgeZero(t *testing.T) {
	directives, _ := ParseCacheControl("immutable, max-age=0")
	diags := crossCheckDirectives(directives)
	if !hasDiagWithMessage(diags, `"immutable" with "max-age=0" is contradictory`) {
		t.Errorf("expected immutable/max-age=0 diagnostic, got %#v", diags)
	}

	directives, _ = ParseCacheControl("immutable, max-age=60")
	diags = crossCheckDirectives(directives)
	if hasDiagWithMessage(diags, `"immutable" with "max-age=0" is contradictory`) {
		t.Errorf("immutable with nonzero max-age should not warn, got %#v", diags)
	}
}

func TestCrossCheckNoCacheNoStoreRedundant(t *testing.T) {
	directives, _ := ParseCacheControl("no-cache, no-store")
	diags := crossCheckDirectives(directives)
	if !hasDiagWithMessage(diags, `"no-store" already implies what "no-cache" does`) {
		t.Errorf("expected no-cache/no-store redundancy diagnostic, got %#v", diags)
	}
}

func TestCrossCheckNoConflicts(t *testing.T) {
	directives, _ := ParseCacheControl("public, max-age=3600")
	diags := crossCheckDirectives(directives)
	if len(diags) != 0 {
		t.Errorf("expected no cross-check diagnostics, got %#v", diags)
	}
}

func hasDiagWithMessage(diags []Diagnostic, msg string) bool {
	for _, d := range diags {
		if d.Message == msg {
			return true
		}
	}
	return false
}

func hasDiagWithSeverity(diags []Diagnostic, sev Severity) bool {
	for _, d := range diags {
		if d.Severity == sev {
			return true
		}
	}
	return false
}
