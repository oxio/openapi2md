package openapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// Parse decodes an OpenAPI 3.0 or 3.1 document from JSON or YAML bytes
// and validates that required top-level fields are present.
func Parse(data []byte) (*Document, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("decode OpenAPI document: empty input")
	}

	var jsonBytes []byte
	if trimmed[0] == '{' {
		jsonBytes = trimmed
	} else {
		var err error
		jsonBytes, err = yamlToJSON(trimmed)
		if err != nil {
			return nil, fmt.Errorf("decode OpenAPI document: %w", err)
		}
	}

	var doc Document
	if err := json.Unmarshal(jsonBytes, &doc); err != nil {
		return nil, fmt.Errorf("decode OpenAPI document: %w", err)
	}

	if strings.TrimSpace(doc.OpenAPI) == "" {
		return nil, fmt.Errorf("missing openapi version")
	}
	if strings.TrimSpace(doc.Info.Title) == "" {
		return nil, fmt.Errorf("missing info.title")
	}
	if len(doc.Paths) == 0 && len(doc.Webhooks) == 0 {
		return nil, fmt.Errorf("OpenAPI document defines no paths or webhooks")
	}

	return &doc, nil
}

// yamlToJSON converts a YAML document to JSON bytes, stringifying any non-string
// mapping keys (such as numeric HTTP status codes like 200 or 404) and resolving
// YAML merge keys and aliases.
func yamlToJSON(data []byte) ([]byte, error) {
	var node yaml.Node
	if err := yaml.Unmarshal(data, &node); err != nil {
		return nil, err
	}
	if node.Kind == 0 {
		return nil, fmt.Errorf("empty YAML document")
	}
	val, err := yamlNodeToValue(&node)
	if err != nil {
		return nil, err
	}
	return json.Marshal(val)
}

func yamlNodeToValue(n *yaml.Node) (any, error) {
	if n == nil {
		return nil, nil
	}
	switch n.Kind {
	case yaml.DocumentNode:
		if len(n.Content) == 0 {
			return nil, nil
		}
		return yamlNodeToValue(n.Content[0])
	case yaml.AliasNode:
		return yamlNodeToValue(n.Alias)
	case yaml.MappingNode:
		m := make(map[string]any, len(n.Content)/2)
		for i := 0; i+1 < len(n.Content); i += 2 {
			kNode := n.Content[i]
			vNode := n.Content[i+1]

			// Handle YAML merge key (<<)
			if kNode.Kind == yaml.ScalarNode && kNode.Tag == "!!merge" {
				if err := applyYAMLMerge(m, vNode); err != nil {
					return nil, err
				}
				continue
			}

			key := scalarKey(kNode)
			val, err := yamlNodeToValue(vNode)
			if err != nil {
				return nil, err
			}
			m[key] = val
		}
		return m, nil
	case yaml.SequenceNode:
		arr := make([]any, 0, len(n.Content))
		for _, item := range n.Content {
			val, err := yamlNodeToValue(item)
			if err != nil {
				return nil, err
			}
			arr = append(arr, val)
		}
		return arr, nil
	case yaml.ScalarNode:
		var v any
		if err := n.Decode(&v); err != nil {
			return nil, err
		}
		return v, nil
	default:
		return nil, nil
	}
}

func applyYAMLMerge(dst map[string]any, vNode *yaml.Node) error {
	if vNode.Kind == yaml.AliasNode {
		vNode = vNode.Alias
	}
	switch vNode.Kind {
	case yaml.MappingNode:
		merged, err := yamlNodeToValue(vNode)
		if err != nil {
			return err
		}
		if mm, ok := merged.(map[string]any); ok {
			for k, v := range mm {
				if _, exists := dst[k]; !exists {
					dst[k] = v
				}
			}
		}
	case yaml.SequenceNode:
		for _, item := range vNode.Content {
			if err := applyYAMLMerge(dst, item); err != nil {
				return err
			}
		}
	}
	return nil
}

func scalarKey(n *yaml.Node) string {
	if n.Kind == yaml.AliasNode && n.Alias != nil {
		n = n.Alias
	}
	return n.Value
}

// RefName extracts the component name from a local JSON Pointer $ref
// with the given prefix (e.g. "#/components/schemas/").
func RefName(ref, prefix string) (string, bool) {
	if !strings.HasPrefix(ref, prefix) {
		return "", false
	}
	raw := strings.TrimPrefix(ref, prefix)
	if raw == "" || strings.Contains(raw, "/") {
		return "", false
	}
	return unescapeJSONPointer(raw), true
}

func unescapeJSONPointer(s string) string {
	s = strings.ReplaceAll(s, "~1", "/")
	s = strings.ReplaceAll(s, "~0", "~")
	return s
}

// SchemaRefName returns the schema name if ref points to #/components/schemas/<name>.
func SchemaRefName(ref string) (string, bool) {
	return RefName(ref, "#/components/schemas/")
}

// ResolveParameter resolves local #/components/parameters/<name> references.
func (d *Document) ResolveParameter(p Parameter) Parameter {
	seen := make(map[string]bool)
	for p.Ref != "" && !seen[p.Ref] {
		seen[p.Ref] = true
		name, ok := RefName(p.Ref, "#/components/parameters/")
		if !ok || d.Components == nil {
			break
		}
		target, found := d.Components.Parameters[name]
		if !found {
			break
		}
		// Preserve any sibling description override (OpenAPI 3.1)
		desc := p.Description
		p = target
		if desc != "" {
			p.Description = desc
		}
	}
	return p
}

// ResolveRequestBody resolves local #/components/requestBodies/<name> references.
func (d *Document) ResolveRequestBody(rb *RequestBody) *RequestBody {
	if rb == nil {
		return nil
	}
	cur := *rb
	seen := make(map[string]bool)
	for cur.Ref != "" && !seen[cur.Ref] {
		seen[cur.Ref] = true
		name, ok := RefName(cur.Ref, "#/components/requestBodies/")
		if !ok || d.Components == nil {
			break
		}
		target, found := d.Components.RequestBodies[name]
		if !found {
			break
		}
		desc := cur.Description
		cur = target
		if desc != "" {
			cur.Description = desc
		}
	}
	return &cur
}

// ResolveResponse resolves local #/components/responses/<name> references.
func (d *Document) ResolveResponse(r Response) Response {
	seen := make(map[string]bool)
	for r.Ref != "" && !seen[r.Ref] {
		seen[r.Ref] = true
		name, ok := RefName(r.Ref, "#/components/responses/")
		if !ok || d.Components == nil {
			break
		}
		target, found := d.Components.Responses[name]
		if !found {
			break
		}
		desc := r.Description
		r = target
		if desc != "" {
			r.Description = desc
		}
	}
	return r
}

// ResolveHeader resolves local #/components/headers/<name> references.
func (d *Document) ResolveHeader(h Header) Header {
	seen := make(map[string]bool)
	for h.Ref != "" && !seen[h.Ref] {
		seen[h.Ref] = true
		name, ok := RefName(h.Ref, "#/components/headers/")
		if !ok || d.Components == nil {
			break
		}
		target, found := d.Components.Headers[name]
		if !found {
			break
		}
		desc := h.Description
		h = target
		if desc != "" {
			h.Description = desc
		}
	}
	return h
}

// ResolveExample resolves local #/components/examples/<name> references.
func (d *Document) ResolveExample(e Example) Example {
	seen := make(map[string]bool)
	for e.Ref != "" && !seen[e.Ref] {
		seen[e.Ref] = true
		name, ok := RefName(e.Ref, "#/components/examples/")
		if !ok || d.Components == nil {
			break
		}
		target, found := d.Components.Examples[name]
		if !found {
			break
		}
		e = target
	}
	return e
}

// ResolveSecurityScheme resolves local #/components/securitySchemes/<name> references.
func (d *Document) ResolveSecurityScheme(s SecurityScheme) SecurityScheme {
	seen := make(map[string]bool)
	for s.Ref != "" && !seen[s.Ref] {
		seen[s.Ref] = true
		name, ok := RefName(s.Ref, "#/components/securitySchemes/")
		if !ok || d.Components == nil {
			break
		}
		target, found := d.Components.SecuritySchemes[name]
		if !found {
			break
		}
		s = target
	}
	return s
}

// ResolvePathItem resolves local #/components/pathItems/<name> references.
func (d *Document) ResolvePathItem(pi PathItem) PathItem {
	seen := make(map[string]bool)
	for pi.Ref != "" && !seen[pi.Ref] {
		seen[pi.Ref] = true
		name, ok := RefName(pi.Ref, "#/components/pathItems/")
		if !ok || d.Components == nil {
			break
		}
		target, found := d.Components.PathItems[name]
		if !found {
			break
		}
		pi = target
	}
	return pi
}

// ResolveCallback resolves local #/components/callbacks/<name> references.
func (d *Document) ResolveCallback(c Callback) Callback {
	seen := make(map[string]bool)
	for c.Ref != "" && !seen[c.Ref] {
		seen[c.Ref] = true
		name, ok := RefName(c.Ref, "#/components/callbacks/")
		if !ok || d.Components == nil {
			break
		}
		target, found := d.Components.Callbacks[name]
		if !found {
			break
		}
		c = target
	}
	return c
}

// LookupSchema returns the component schema for a local #/components/schemas/<name> ref.
func (d *Document) LookupSchema(ref string) (*Schema, string, bool) {
	name, ok := SchemaRefName(ref)
	if !ok || d.Components == nil {
		return nil, "", false
	}
	s, found := d.Components.Schemas[name]
	return s, name, found
}
