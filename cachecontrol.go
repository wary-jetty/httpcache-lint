package main

import (
	"strconv"
	"strings"
)

// ValueKind describes what shape a Cache-Control directive's value is
// expected to take, per RFC 9111 section 5.2.
type ValueKind int

const (
	KindNone                 ValueKind = iota // flag directive, no "=value"
	KindDeltaSeconds                           // "=N", required
	KindOptionalDeltaSeconds                   // "=N", optional
	KindOptionalFieldList                      // optional "=\"field, field\""
)

type directiveSpec struct {
	kind ValueKind
}

var knownDirectives = map[string]directiveSpec{
	"no-store":               {KindNone},
	"no-transform":           {KindNone},
	"must-revalidate":        {KindNone},
	"proxy-revalidate":       {KindNone},
	"public":                 {KindNone},
	"immutable":              {KindNone},
	"only-if-cached":         {KindNone},
	"max-age":                {KindDeltaSeconds},
	"s-maxage":                {KindDeltaSeconds},
	"min-fresh":               {KindDeltaSeconds},
	"stale-while-revalidate":  {KindDeltaSeconds},
	"stale-if-error":          {KindDeltaSeconds},
	"max-stale":               {KindOptionalDeltaSeconds},
	"no-cache":                {KindOptionalFieldList},
	"private":                 {KindOptionalFieldList},
}

type Directive struct {
	Name     string `json:"name"`
	Value    string `json:"value,omitempty"`
	HasValue bool   `json:"hasValue"`
}

// splitCacheControl splits a Cache-Control header value on top-level commas,
// leaving commas inside quoted-strings alone (e.g. no-cache="set-cookie,x-foo").
func splitCacheControl(raw string) []string {
	var parts []string
	var cur strings.Builder
	inQuotes := false
	for _, r := range raw {
		switch {
		case r == '"':
			inQuotes = !inQuotes
			cur.WriteRune(r)
		case r == ',' && !inQuotes:
			parts = append(parts, cur.String())
			cur.Reset()
		default:
			cur.WriteRune(r)
		}
	}
	parts = append(parts, cur.String())
	return parts
}

func parseDirective(part string) Directive {
	part = strings.TrimSpace(part)
	name, value, hasValue := part, "", false
	if idx := strings.IndexByte(part, '='); idx >= 0 {
		name = strings.TrimSpace(part[:idx])
		value = strings.TrimSpace(part[idx+1:])
		hasValue = true
	}
	return Directive{Name: strings.ToLower(name), Value: value, HasValue: hasValue}
}

// ParseCacheControl parses a Cache-Control header value into directives plus
// validation diagnostics. It never fails outright on bad input: malformed
// directives become diagnostics rather than a parse error, since showing
// what's wrong is the whole point of the tool.
func ParseCacheControl(raw string) ([]Directive, []Diagnostic) {
	var directives []Directive
	var diags []Diagnostic
	seen := map[string]int{}

	for _, part := range splitCacheControl(raw) {
		// RFC 9111's list syntax tolerates empty elements between commas
		// (trailing commas, doubled commas); they're not an error.
		if strings.TrimSpace(part) == "" {
			continue
		}
		d := parseDirective(part)
		directives = append(directives, d)
		seen[d.Name]++
		if seen[d.Name] == 2 {
			diags = append(diags, Diagnostic{Severity: SevWarning, Header: "Cache-Control", Message: "directive \"" + d.Name + "\" repeated"})
		}

		spec, known := knownDirectives[d.Name]
		if !known {
			diags = append(diags, Diagnostic{Severity: SevWarning, Header: "Cache-Control", Message: "unrecognized directive \"" + d.Name + "\""})
			continue
		}
		diags = append(diags, validateDirectiveValue(d, spec)...)
	}

	diags = append(diags, crossCheckDirectives(directives)...)
	return directives, diags
}

func validateDirectiveValue(d Directive, spec directiveSpec) []Diagnostic {
	var diags []Diagnostic
	switch spec.kind {
	case KindNone:
		if d.HasValue {
			diags = append(diags, Diagnostic{Severity: SevWarning, Header: "Cache-Control", Message: "directive \"" + d.Name + "\" does not take a value, got \"" + d.Value + "\""})
		}
	case KindDeltaSeconds:
		if !d.HasValue {
			diags = append(diags, Diagnostic{Severity: SevError, Header: "Cache-Control", Message: "directive \"" + d.Name + "\" requires a delta-seconds value"})
			break
		}
		diags = append(diags, checkDeltaSeconds(d)...)
	case KindOptionalDeltaSeconds:
		if d.HasValue {
			diags = append(diags, checkDeltaSeconds(d)...)
		}
	case KindOptionalFieldList:
		if d.HasValue && !looksLikeFieldList(d.Value) {
			diags = append(diags, Diagnostic{Severity: SevWarning, Header: "Cache-Control", Message: "directive \"" + d.Name + "\" value should be a quoted, comma-separated field list, got \"" + d.Value + "\""})
		}
	}
	return diags
}

func checkDeltaSeconds(d Directive) []Diagnostic {
	n, err := strconv.Atoi(d.Value)
	if err != nil {
		return []Diagnostic{{Severity: SevError, Header: "Cache-Control", Message: "directive \"" + d.Name + "\" value \"" + d.Value + "\" is not an integer number of seconds"}}
	}
	if n < 0 {
		return []Diagnostic{{Severity: SevError, Header: "Cache-Control", Message: "directive \"" + d.Name + "\" must not be negative"}}
	}
	return nil
}

func looksLikeFieldList(value string) bool {
	value = strings.TrimSpace(value)
	if len(value) < 2 || value[0] != '"' || value[len(value)-1] != '"' {
		return false
	}
	inner := value[1 : len(value)-1]
	for _, field := range strings.Split(inner, ",") {
		if strings.TrimSpace(field) == "" {
			return false
		}
	}
	return true
}

func hasDirective(directives []Directive, name string) (Directive, bool) {
	for _, d := range directives {
		if d.Name == name {
			return d, true
		}
	}
	return Directive{}, false
}

// crossCheckDirectives catches combinations that are individually valid but
// contradictory or redundant together.
func crossCheckDirectives(directives []Directive) []Diagnostic {
	var diags []Diagnostic

	_, public := hasDirective(directives, "public")
	_, private := hasDirective(directives, "private")
	if public && private {
		diags = append(diags, Diagnostic{Severity: SevError, Header: "Cache-Control", Message: "\"public\" and \"private\" are mutually exclusive"})
	}

	_, noStore := hasDirective(directives, "no-store")
	if maxAge, ok := hasDirective(directives, "max-age"); ok && noStore {
		diags = append(diags, Diagnostic{Severity: SevWarning, Header: "Cache-Control", Message: "\"no-store\" makes \"max-age=" + maxAge.Value + "\" pointless"})
	}

	if maxAge, ok := hasDirective(directives, "max-age"); ok {
		if _, immutable := hasDirective(directives, "immutable"); immutable && maxAge.Value == "0" {
			diags = append(diags, Diagnostic{Severity: SevWarning, Header: "Cache-Control", Message: "\"immutable\" with \"max-age=0\" is contradictory"})
		}
	}

	if _, noCache := hasDirective(directives, "no-cache"); noCache && noStore {
		diags = append(diags, Diagnostic{Severity: SevInfo, Header: "Cache-Control", Message: "\"no-store\" already implies what \"no-cache\" does"})
	}

	return diags
}
