package openapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// Document represents an OpenAPI 3.0 or 3.1 specification document.
type Document struct {
	OpenAPI           string                `json:"openapi"`
	JSONSchemaDialect string                `json:"jsonSchemaDialect,omitempty"`
	Info              Info                  `json:"info"`
	Servers           []Server              `json:"servers,omitempty"`
	Paths             map[string]PathItem   `json:"paths,omitempty"`
	Webhooks          map[string]PathItem   `json:"webhooks,omitempty"`
	Components        *Components           `json:"components,omitempty"`
	Security          []SecurityRequirement `json:"security,omitempty"`
	Tags              []Tag                 `json:"tags,omitempty"`
	ExternalDocs      *ExternalDocs         `json:"externalDocs,omitempty"`
}

// Info holds API metadata.
type Info struct {
	Title          string   `json:"title"`
	Summary        string   `json:"summary,omitempty"`
	Description    string   `json:"description,omitempty"`
	TermsOfService string   `json:"termsOfService,omitempty"`
	Contact        *Contact `json:"contact,omitempty"`
	License        *License `json:"license,omitempty"`
	Version        string   `json:"version,omitempty"`
}

// Contact holds contact information for the exposed API.
type Contact struct {
	Name  string `json:"name,omitempty"`
	URL   string `json:"url,omitempty"`
	Email string `json:"email,omitempty"`
}

// License holds license information for the exposed API.
type License struct {
	Name       string `json:"name"`
	Identifier string `json:"identifier,omitempty"`
	URL        string `json:"url,omitempty"`
}

// Server represents a server entry.
type Server struct {
	URL         string                    `json:"url"`
	Description string                    `json:"description,omitempty"`
	Variables   map[string]ServerVariable `json:"variables,omitempty"`
}

// ServerVariable represents a server URL template variable.
type ServerVariable struct {
	Enum        []string `json:"enum,omitempty"`
	Default     string   `json:"default"`
	Description string   `json:"description,omitempty"`
}

// Tag represents an OpenAPI tag definition.
type Tag struct {
	Name         string        `json:"name"`
	Description  string        `json:"description,omitempty"`
	ExternalDocs *ExternalDocs `json:"externalDocs,omitempty"`
}

// ExternalDocs references external documentation.
type ExternalDocs struct {
	Description string `json:"description,omitempty"`
	URL         string `json:"url"`
}

// SecurityRequirement maps security scheme names to required scopes.
type SecurityRequirement map[string][]string

// HTTPMethods defines the canonical display ordering of HTTP methods.
var HTTPMethods = []string{"get", "head", "post", "put", "patch", "delete", "options", "trace"}

// PathItem describes the operations available on a single path.
type PathItem struct {
	Ref         string                     `json:"$ref,omitempty"`
	Summary     string                     `json:"summary,omitempty"`
	Description string                     `json:"description,omitempty"`
	Servers     []Server                   `json:"servers,omitempty"`
	Parameters  []Parameter                `json:"parameters,omitempty"`
	RawOps      map[string]json.RawMessage `json:"-"`
}

// UnmarshalJSON decodes a PathItem while keeping individual HTTP operations as
// raw JSON so that malformed operations can be reported with their method and path.
func (p *PathItem) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if v, ok := raw["$ref"]; ok {
		if err := json.Unmarshal(v, &p.Ref); err != nil {
			return err
		}
	}
	if v, ok := raw["summary"]; ok {
		if err := json.Unmarshal(v, &p.Summary); err != nil {
			return err
		}
	}
	if v, ok := raw["description"]; ok {
		if err := json.Unmarshal(v, &p.Description); err != nil {
			return err
		}
	}
	if v, ok := raw["servers"]; ok {
		if err := json.Unmarshal(v, &p.Servers); err != nil {
			return err
		}
	}
	if v, ok := raw["parameters"]; ok {
		if err := json.Unmarshal(v, &p.Parameters); err != nil {
			return err
		}
	}
	for _, method := range HTTPMethods {
		if v, ok := raw[method]; ok && len(bytes.TrimSpace(v)) > 0 && string(bytes.TrimSpace(v)) != "null" {
			if p.RawOps == nil {
				p.RawOps = make(map[string]json.RawMessage)
			}
			p.RawOps[method] = v
		}
	}
	return nil
}

// DecodeOperation decodes the operation for the given lowercase HTTP method.
func (p *PathItem) DecodeOperation(method, path string) (*Operation, error) {
	raw, ok := p.RawOps[method]
	if !ok {
		return nil, nil
	}
	var op Operation
	if err := json.Unmarshal(raw, &op); err != nil {
		return nil, fmt.Errorf("decode %s %s: %w", strings.ToUpper(method), path, err)
	}
	return &op, nil
}

// Operation describes a single API operation on a path.
type Operation struct {
	Tags         []string               `json:"tags,omitempty"`
	Summary      string                 `json:"summary,omitempty"`
	Description  string                 `json:"description,omitempty"`
	ExternalDocs *ExternalDocs          `json:"externalDocs,omitempty"`
	OperationID  string                 `json:"operationId,omitempty"`
	Parameters   []Parameter            `json:"parameters,omitempty"`
	RequestBody  *RequestBody           `json:"requestBody,omitempty"`
	Responses    map[string]Response    `json:"responses,omitempty"`
	Callbacks    map[string]Callback    `json:"callbacks,omitempty"`
	Deprecated   bool                   `json:"deprecated,omitempty"`
	Security     *[]SecurityRequirement `json:"security,omitempty"`
	Servers      []Server               `json:"servers,omitempty"`
}

// Callback maps runtime expressions to PathItems (or $ref).
type Callback struct {
	Ref   string              `json:"$ref,omitempty"`
	Paths map[string]PathItem `json:"-"`
}

// UnmarshalJSON decodes a Callback object.
func (c *Callback) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	for k, v := range raw {
		if k == "$ref" {
			if err := json.Unmarshal(v, &c.Ref); err != nil {
				return err
			}
			continue
		}
		if strings.HasPrefix(k, "x-") {
			continue
		}
		var pi PathItem
		if err := json.Unmarshal(v, &pi); err != nil {
			return err
		}
		if c.Paths == nil {
			c.Paths = make(map[string]PathItem)
		}
		c.Paths[k] = pi
	}
	return nil
}

// Parameter describes a single operation parameter.
type Parameter struct {
	Ref             string               `json:"$ref,omitempty"`
	Name            string               `json:"name,omitempty"`
	In              string               `json:"in,omitempty"`
	Description     string               `json:"description,omitempty"`
	Required        bool                 `json:"required,omitempty"`
	Deprecated      bool                 `json:"deprecated,omitempty"`
	AllowEmptyValue bool                 `json:"allowEmptyValue,omitempty"`
	Style           string               `json:"style,omitempty"`
	Explode         *bool                `json:"explode,omitempty"`
	Schema          *Schema              `json:"schema,omitempty"`
	Example         any                  `json:"example,omitempty"`
	HasExample      bool                 `json:"-"`
	Examples        map[string]Example   `json:"examples,omitempty"`
	Content         map[string]MediaType `json:"content,omitempty"`
}

// UnmarshalJSON decodes Parameter while tracking explicit example presence.
func (p *Parameter) UnmarshalJSON(data []byte) error {
	type alias Parameter
	var a alias
	if err := json.Unmarshal(data, &a); err != nil {
		return err
	}
	*p = Parameter(a)
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err == nil {
		if _, ok := raw["example"]; ok {
			p.HasExample = true
		}
	}
	return nil
}

// RequestBody describes a single request body.
type RequestBody struct {
	Ref         string               `json:"$ref,omitempty"`
	Description string               `json:"description,omitempty"`
	Content     map[string]MediaType `json:"content,omitempty"`
	Required    bool                 `json:"required,omitempty"`
}

// Response describes a single response from an API operation.
type Response struct {
	Ref         string               `json:"$ref,omitempty"`
	Description string               `json:"description,omitempty"`
	Headers     map[string]Header    `json:"headers,omitempty"`
	Content     map[string]MediaType `json:"content,omitempty"`
}

// Header describes a single response header.
type Header struct {
	Ref         string               `json:"$ref,omitempty"`
	Description string               `json:"description,omitempty"`
	Required    bool                 `json:"required,omitempty"`
	Deprecated  bool                 `json:"deprecated,omitempty"`
	Schema      *Schema              `json:"schema,omitempty"`
	Example     any                  `json:"example,omitempty"`
	HasExample  bool                 `json:"-"`
	Examples    map[string]Example   `json:"examples,omitempty"`
	Content     map[string]MediaType `json:"content,omitempty"`
}

// UnmarshalJSON decodes Header while tracking explicit example presence.
func (h *Header) UnmarshalJSON(data []byte) error {
	type alias Header
	var a alias
	if err := json.Unmarshal(data, &a); err != nil {
		return err
	}
	*h = Header(a)
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err == nil {
		if _, ok := raw["example"]; ok {
			h.HasExample = true
		}
	}
	return nil
}

// MediaType provides schema and examples for a media type.
type MediaType struct {
	Schema     *Schema            `json:"schema,omitempty"`
	Example    any                `json:"example,omitempty"`
	HasExample bool               `json:"-"`
	Examples   map[string]Example `json:"examples,omitempty"`
}

// UnmarshalJSON decodes MediaType while tracking explicit example presence.
func (m *MediaType) UnmarshalJSON(data []byte) error {
	type alias MediaType
	var a alias
	if err := json.Unmarshal(data, &a); err != nil {
		return err
	}
	*m = MediaType(a)
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err == nil {
		if _, ok := raw["example"]; ok {
			m.HasExample = true
		}
	}
	return nil
}

// Example represents an OpenAPI Example Object.
type Example struct {
	Ref           string `json:"$ref,omitempty"`
	Summary       string `json:"summary,omitempty"`
	Description   string `json:"description,omitempty"`
	Value         any    `json:"value,omitempty"`
	HasValue      bool   `json:"-"`
	ExternalValue string `json:"externalValue,omitempty"`
}

// UnmarshalJSON decodes Example while tracking explicit value presence.
func (e *Example) UnmarshalJSON(data []byte) error {
	type alias Example
	var a alias
	if err := json.Unmarshal(data, &a); err != nil {
		return err
	}
	*e = Example(a)
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err == nil {
		if _, ok := raw["value"]; ok {
			e.HasValue = true
		}
	}
	return nil
}

// Components holds reusable OpenAPI objects.
type Components struct {
	Schemas         map[string]*Schema        `json:"schemas,omitempty"`
	Responses       map[string]Response       `json:"responses,omitempty"`
	Parameters      map[string]Parameter      `json:"parameters,omitempty"`
	Examples        map[string]Example        `json:"examples,omitempty"`
	RequestBodies   map[string]RequestBody    `json:"requestBodies,omitempty"`
	Headers         map[string]Header         `json:"headers,omitempty"`
	SecuritySchemes map[string]SecurityScheme `json:"securitySchemes,omitempty"`
	Callbacks       map[string]Callback       `json:"callbacks,omitempty"`
	PathItems       map[string]PathItem       `json:"pathItems,omitempty"`
}

// SecurityScheme defines a security scheme used by operations.
type SecurityScheme struct {
	Ref              string      `json:"$ref,omitempty"`
	Type             string      `json:"type,omitempty"`
	Description      string      `json:"description,omitempty"`
	Name             string      `json:"name,omitempty"`
	In               string      `json:"in,omitempty"`
	Scheme           string      `json:"scheme,omitempty"`
	BearerFormat     string      `json:"bearerFormat,omitempty"`
	Flows            *OAuthFlows `json:"flows,omitempty"`
	OpenIDConnectURL string      `json:"openIdConnectUrl,omitempty"`
}

// OAuthFlows configures supported OAuth2 flows.
type OAuthFlows struct {
	Implicit          *OAuthFlow `json:"implicit,omitempty"`
	Password          *OAuthFlow `json:"password,omitempty"`
	ClientCredentials *OAuthFlow `json:"clientCredentials,omitempty"`
	AuthorizationCode *OAuthFlow `json:"authorizationCode,omitempty"`
}

// OAuthFlow configures a single OAuth2 flow.
type OAuthFlow struct {
	AuthorizationURL string            `json:"authorizationUrl,omitempty"`
	TokenURL         string            `json:"tokenUrl,omitempty"`
	RefreshURL       string            `json:"refreshUrl,omitempty"`
	Scopes           map[string]string `json:"scopes,omitempty"`
}

// SchemaTypes supports both OpenAPI 3.0 string ("string") and OpenAPI 3.1
// string array (["array", "null"]).
type SchemaTypes []string

// UnmarshalJSON decodes a single type string or an array of type strings.
func (t *SchemaTypes) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || string(trimmed) == "null" {
		return nil
	}
	if trimmed[0] == '"' {
		var s string
		if err := json.Unmarshal(trimmed, &s); err != nil {
			return err
		}
		*t = SchemaTypes{s}
		return nil
	}
	var list []string
	if err := json.Unmarshal(trimmed, &list); err != nil {
		return err
	}
	*t = SchemaTypes(list)
	return nil
}

// AdditionalProperties supports both boolean (true/false) and *Schema.
type AdditionalProperties struct {
	Allowed *bool
	Schema  *Schema
}

// UnmarshalJSON decodes additionalProperties as either a bool or a Schema.
func (a *AdditionalProperties) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || string(trimmed) == "null" {
		return nil
	}
	if string(trimmed) == "true" || string(trimmed) == "false" {
		b := string(trimmed) == "true"
		a.Allowed = &b
		return nil
	}
	var s Schema
	if err := json.Unmarshal(trimmed, &s); err != nil {
		return err
	}
	a.Schema = &s
	return nil
}

// ExclusiveBound supports both OpenAPI 3.0 boolean and OpenAPI 3.1 numeric
// exclusiveMinimum / exclusiveMaximum values.
type ExclusiveBound struct {
	Bool   *bool
	Number *float64
}

// UnmarshalJSON decodes either a boolean or a number.
func (b *ExclusiveBound) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || string(trimmed) == "null" {
		return nil
	}
	if string(trimmed) == "true" || string(trimmed) == "false" {
		v := string(trimmed) == "true"
		b.Bool = &v
		return nil
	}
	var n float64
	if err := json.Unmarshal(trimmed, &n); err != nil {
		return err
	}
	b.Number = &n
	return nil
}

// Discriminator aids serialization/deserialization of oneOf/anyOf schemas.
type Discriminator struct {
	PropertyName string            `json:"propertyName"`
	Mapping      map[string]string `json:"mapping,omitempty"`
}

// Schema represents an OpenAPI 3.0 / 3.1 / JSON Schema 2020-12 schema object
// (including boolean schemas true/false).
type Schema struct {
	Boolean *bool `json:"-"`

	Ref         string `json:"$ref,omitempty"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`

	Types    SchemaTypes `json:"type,omitempty"`
	Format   string      `json:"format,omitempty"`
	Nullable bool        `json:"nullable,omitempty"`

	Default    any   `json:"default,omitempty"`
	HasDefault bool  `json:"-"`
	Example    any   `json:"example,omitempty"`
	HasExample bool  `json:"-"`
	Examples   []any `json:"examples,omitempty"`
	Enum       []any `json:"enum,omitempty"`
	Const      any   `json:"const,omitempty"`
	HasConst   bool  `json:"-"`

	Required             []string              `json:"required,omitempty"`
	Properties           map[string]*Schema    `json:"properties,omitempty"`
	AdditionalProperties *AdditionalProperties `json:"additionalProperties,omitempty"`

	Items       *Schema   `json:"items,omitempty"`
	PrefixItems []*Schema `json:"prefixItems,omitempty"`

	AllOf         []*Schema      `json:"allOf,omitempty"`
	OneOf         []*Schema      `json:"oneOf,omitempty"`
	AnyOf         []*Schema      `json:"anyOf,omitempty"`
	Not           *Schema        `json:"not,omitempty"`
	Discriminator *Discriminator `json:"discriminator,omitempty"`

	ReadOnly   bool `json:"readOnly,omitempty"`
	WriteOnly  bool `json:"writeOnly,omitempty"`
	Deprecated bool `json:"deprecated,omitempty"`

	MultipleOf       *float64        `json:"multipleOf,omitempty"`
	Maximum          *float64        `json:"maximum,omitempty"`
	Minimum          *float64        `json:"minimum,omitempty"`
	ExclusiveMaximum *ExclusiveBound `json:"exclusiveMaximum,omitempty"`
	ExclusiveMinimum *ExclusiveBound `json:"exclusiveMinimum,omitempty"`
	MaxLength        *int            `json:"maxLength,omitempty"`
	MinLength        *int            `json:"minLength,omitempty"`
	Pattern          string          `json:"pattern,omitempty"`
	MaxItems         *int            `json:"maxItems,omitempty"`
	MinItems         *int            `json:"minItems,omitempty"`
	UniqueItems      bool            `json:"uniqueItems,omitempty"`
	MaxProperties    *int            `json:"maxProperties,omitempty"`
	MinProperties    *int            `json:"minProperties,omitempty"`

	ContentMediaType string        `json:"contentMediaType,omitempty"`
	ContentEncoding  string        `json:"contentEncoding,omitempty"`
	ExternalDocs     *ExternalDocs `json:"externalDocs,omitempty"`
}

// UnmarshalJSON decodes a Schema, handling JSON Schema boolean schemas (true/false)
// and tracking explicit presence of default, example, and const.
func (s *Schema) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || string(trimmed) == "null" {
		return nil
	}
	if string(trimmed) == "true" || string(trimmed) == "false" {
		b := string(trimmed) == "true"
		s.Boolean = &b
		return nil
	}
	type alias Schema
	var a alias
	if err := json.Unmarshal(trimmed, &a); err != nil {
		return err
	}
	*s = Schema(a)

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(trimmed, &raw); err == nil {
		if _, ok := raw["default"]; ok {
			s.HasDefault = true
		}
		if _, ok := raw["example"]; ok {
			s.HasExample = true
		}
		if _, ok := raw["const"]; ok {
			s.HasConst = true
		}
	}
	return nil
}

// IsEmpty reports whether the schema imposes no type or composition constraints
// (representing "any" in OpenAPI / JSON Schema).
func (s *Schema) IsEmpty() bool {
	if s == nil {
		return true
	}
	if s.Boolean != nil {
		return *s.Boolean
	}
	return s.Ref == "" &&
		len(s.Types) == 0 &&
		len(s.Properties) == 0 &&
		s.AdditionalProperties == nil &&
		s.Items == nil &&
		len(s.PrefixItems) == 0 &&
		len(s.AllOf) == 0 &&
		len(s.OneOf) == 0 &&
		len(s.AnyOf) == 0 &&
		s.Not == nil &&
		len(s.Enum) == 0 &&
		!s.HasConst
}
