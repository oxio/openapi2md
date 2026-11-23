package markdown

import (
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/oxio/openapi2md/internal/openapi"
)

const maxSchemaDepth = 8

// formatType renders a human-readable Markdown type expression for a Schema.
func (r *renderer) formatType(s *openapi.Schema) string {
	return r.formatTypeDepth(s, 0)
}

func (r *renderer) formatTypeDepth(s *openapi.Schema, depth int) string {
	if s == nil {
		return "any"
	}
	if depth > maxSchemaDepth {
		return "any"
	}
	if s.Boolean != nil {
		if *s.Boolean {
			return "any"
		}
		return "never"
	}
	if s.Ref != "" {
		base := r.formatRef(s.Ref)
		if s.Nullable {
			return base + ", nullable"
		}
		return base
	}

	nullable := s.Nullable || slices.Contains(s.Types, "null")

	// Handle oneOf
	if len(s.OneOf) > 0 {
		if other, ok := extractNullableUnion(s.OneOf); ok {
			return appendNullable(r.formatTypeDepth(other, depth+1), true)
		}
		parts := make([]string, 0, len(s.OneOf))
		for _, sub := range s.OneOf {
			parts = append(parts, r.formatTypeDepth(sub, depth+1))
		}
		return appendNullable("one of: "+strings.Join(parts, " | "), nullable)
	}

	// Handle anyOf
	if len(s.AnyOf) > 0 {
		if other, ok := extractNullableUnion(s.AnyOf); ok {
			return appendNullable(r.formatTypeDepth(other, depth+1), true)
		}
		parts := make([]string, 0, len(s.AnyOf))
		for _, sub := range s.AnyOf {
			parts = append(parts, r.formatTypeDepth(sub, depth+1))
		}
		return appendNullable("any of: "+strings.Join(parts, " | "), nullable)
	}

	// Handle allOf
	if len(s.AllOf) > 0 {
		if len(s.AllOf) == 1 && len(s.Properties) == 0 && s.AdditionalProperties == nil {
			return appendNullable(r.formatTypeDepth(s.AllOf[0], depth+1), nullable)
		}
		parts := make([]string, 0, len(s.AllOf))
		for _, sub := range s.AllOf {
			parts = append(parts, r.formatTypeDepth(sub, depth+1))
		}
		return appendNullable("all of: "+strings.Join(parts, " & "), nullable)
	}

	// Handle not
	if s.Not != nil {
		return appendNullable("not "+r.formatTypeDepth(s.Not, depth+1), nullable)
	}

	// Collect non-null types
	nonNull := make([]string, 0, len(s.Types))
	for _, t := range s.Types {
		if t != "null" {
			nonNull = append(nonNull, t)
		}
	}

	// Only "null" type explicitly specified
	if len(nonNull) == 0 && len(s.Types) > 0 {
		return "null"
	}

	// Multiple non-null types (OpenAPI 3.1 type array like ["string", "integer"])
	if len(nonNull) > 1 {
		base := strings.Join(nonNull, " | ")
		if s.Format != "" {
			base += " (" + s.Format + ")"
		}
		if len(s.Enum) > 0 {
			base += " (one of " + formatEnumValues(s.Enum) + ")"
		}
		if s.HasConst {
			base += " (const " + formatJSONValue(s.Const) + ")"
		}
		return appendNullable(base, nullable)
	}

	primaryType := ""
	if len(nonNull) == 1 {
		primaryType = nonNull[0]
	} else {
		// Infer type from structural keywords if type is omitted
		switch {
		case len(s.PrefixItems) > 0 || s.Items != nil:
			primaryType = "array"
		case len(s.Properties) > 0 || s.AdditionalProperties != nil:
			primaryType = "object"
		}
	}

	var base string
	switch primaryType {
	case "array":
		switch {
		case len(s.PrefixItems) > 0:
			elems := make([]string, 0, len(s.PrefixItems))
			for _, item := range s.PrefixItems {
				elems = append(elems, r.formatTypeDepth(item, depth+1))
			}
			base = "tuple of (" + strings.Join(elems, ", ") + ")"
			if s.Items != nil && (s.Items.Boolean == nil || *s.Items.Boolean) {
				base += ", ..." + r.formatTypeDepth(s.Items, depth+1)
			}
		case s.Items != nil:
			base = "array of " + r.formatTypeDepth(s.Items, depth+1)
		default:
			base = "array of any"
		}

	case "object":
		switch {
		case s.AdditionalProperties != nil && s.AdditionalProperties.Schema != nil && len(s.Properties) == 0:
			base = "map of string to " + r.formatTypeDepth(s.AdditionalProperties.Schema, depth+1)
		case len(s.Properties) > 0 && depth < 2 && len(s.Properties) <= 4:
			// Show compact inline fields for small inline objects
			names := sortedKeys(s.Properties)
			fields := make([]string, 0, len(names))
			for _, k := range names {
				fields = append(fields, fmt.Sprintf("`%s`: %s", k, r.formatTypeDepth(s.Properties[k], depth+1)))
			}
			base = "object (" + strings.Join(fields, ", ") + ")"
		default:
			base = "object"
		}

	case "":
		switch {
		case s.HasConst:
			base = "const " + formatJSONValue(s.Const)
		case len(s.Enum) > 0:
			base = "one of " + formatEnumValues(s.Enum)
		default:
			base = "any"
		}

	default:
		base = primaryType
		if s.Format != "" {
			base += " (" + s.Format + ")"
		}
		if s.HasConst {
			base += " (const " + formatJSONValue(s.Const) + ")"
		} else if len(s.Enum) > 0 {
			base += " (one of " + formatEnumValues(s.Enum) + ")"
		}
	}

	return appendNullable(base, nullable)
}

func (r *renderer) formatRef(ref string) string {
	if name, ok := openapi.SchemaRefName(ref); ok {
		if anchor, exists := r.schemaAnchors[name]; exists {
			return fmt.Sprintf("[%s](#%s)", name, anchor)
		}
		return fmt.Sprintf("`%s`", name)
	}
	return fmt.Sprintf("`%s`", ref)
}

func isNullSchema(s *openapi.Schema) bool {
	if s == nil || s.Boolean != nil || s.Ref != "" {
		return false
	}
	return len(s.Types) == 1 && s.Types[0] == "null" &&
		len(s.Properties) == 0 && len(s.OneOf) == 0 && len(s.AnyOf) == 0 && len(s.AllOf) == 0
}

// extractNullableUnion detects the OpenAPI 3.1 nullable pattern:
// anyOf/oneOf with 2 branches where one is {"type": "null"}.
func extractNullableUnion(branches []*openapi.Schema) (*openapi.Schema, bool) {
	if len(branches) != 2 {
		return nil, false
	}
	if isNullSchema(branches[0]) && !isNullSchema(branches[1]) {
		return branches[1], true
	}
	if isNullSchema(branches[1]) && !isNullSchema(branches[0]) {
		return branches[0], true
	}
	return nil, false
}

func appendNullable(base string, nullable bool) string {
	if !nullable || base == "null" || strings.HasSuffix(base, ", nullable") {
		return base
	}
	return base + ", nullable"
}

func formatEnumValues(values []any) string {
	parts := make([]string, 0, len(values))
	for _, v := range values {
		switch tv := v.(type) {
		case string:
			parts = append(parts, "`"+tv+"`")
		default:
			parts = append(parts, formatJSONValue(v))
		}
	}
	return strings.Join(parts, ", ")
}

func formatJSONValue(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("`%v`", v)
	}
	return "`" + string(b) + "`"
}

// formatSchemaDetails builds the description cell / metadata notes for a property
// or parameter schema (description, deprecated, readOnly/writeOnly, default,
// example(s), and validation constraints).
func formatSchemaDetails(desc string, deprecated bool, s *openapi.Schema, paramExample any, hasParamExample bool) string {
	var parts []string

	if deprecated || (s != nil && s.Deprecated) {
		parts = append(parts, "*(Deprecated)*")
	}

	cleanDesc := strings.TrimSpace(desc)
	if cleanDesc == "" && s != nil {
		cleanDesc = strings.TrimSpace(s.Description)
	}
	if cleanDesc != "" {
		parts = append(parts, ensureSentence(cleanDesc))
	}

	if s != nil {
		if s.ReadOnly {
			parts = append(parts, "Read-only.")
		}
		if s.WriteOnly {
			parts = append(parts, "Write-only.")
		}
		if constraints := formatConstraints(s); len(constraints) > 0 {
			parts = append(parts, "Constraints: "+strings.Join(constraints, ", ")+".")
		}
		if s.HasDefault {
			parts = append(parts, "Default: "+formatJSONValue(s.Default)+".")
		}
	}

	// Prefer explicit parameter/header example, then schema.examples, then schema.example.
	switch {
	case hasParamExample:
		parts = append(parts, "Example: "+formatJSONValue(paramExample)+".")
	case s != nil && len(s.Examples) > 0:
		if len(s.Examples) == 1 {
			parts = append(parts, "Example: "+formatJSONValue(s.Examples[0])+".")
		} else {
			ex := make([]string, 0, len(s.Examples))
			for _, e := range s.Examples {
				ex = append(ex, formatJSONValue(e))
			}
			parts = append(parts, "Examples: "+strings.Join(ex, ", ")+".")
		}
	case s != nil && s.HasExample:
		parts = append(parts, "Example: "+formatJSONValue(s.Example)+".")
	}

	return strings.Join(parts, " ")
}

func formatConstraints(s *openapi.Schema) []string {
	var c []string
	if s.MinLength != nil {
		c = append(c, fmt.Sprintf("min length %d", *s.MinLength))
	}
	if s.MaxLength != nil {
		c = append(c, fmt.Sprintf("max length %d", *s.MaxLength))
	}
	if s.Pattern != "" {
		c = append(c, fmt.Sprintf("pattern `%s`", s.Pattern))
	}
	if s.Minimum != nil {
		op := ">="
		if s.ExclusiveMinimum != nil && s.ExclusiveMinimum.Bool != nil && *s.ExclusiveMinimum.Bool {
			op = ">"
		}
		c = append(c, fmt.Sprintf("%s %s", op, formatFloat(*s.Minimum)))
	}
	if s.ExclusiveMinimum != nil && s.ExclusiveMinimum.Number != nil {
		c = append(c, fmt.Sprintf("> %s", formatFloat(*s.ExclusiveMinimum.Number)))
	}
	if s.Maximum != nil {
		op := "<="
		if s.ExclusiveMaximum != nil && s.ExclusiveMaximum.Bool != nil && *s.ExclusiveMaximum.Bool {
			op = "<"
		}
		c = append(c, fmt.Sprintf("%s %s", op, formatFloat(*s.Maximum)))
	}
	if s.ExclusiveMaximum != nil && s.ExclusiveMaximum.Number != nil {
		c = append(c, fmt.Sprintf("< %s", formatFloat(*s.ExclusiveMaximum.Number)))
	}
	if s.MultipleOf != nil {
		c = append(c, fmt.Sprintf("multiple of %s", formatFloat(*s.MultipleOf)))
	}
	if s.MinItems != nil {
		c = append(c, fmt.Sprintf("min items %d", *s.MinItems))
	}
	if s.MaxItems != nil {
		c = append(c, fmt.Sprintf("max items %d", *s.MaxItems))
	}
	if s.UniqueItems {
		c = append(c, "unique items")
	}
	if s.MinProperties != nil {
		c = append(c, fmt.Sprintf("min properties %d", *s.MinProperties))
	}
	if s.MaxProperties != nil {
		c = append(c, fmt.Sprintf("max properties %d", *s.MaxProperties))
	}
	return c
}

func formatFloat(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}

func ensureSentence(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	last := s[len(s)-1]
	if last == '.' || last == '!' || last == '?' || last == ':' {
		return s
	}
	return s + "."
}

// escapeCell escapes pipe characters and newlines so a string is safe inside a
// Markdown table cell.
func escapeCell(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\n", "<br>")
	s = strings.ReplaceAll(s, "|", `\|`)
	return s
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
