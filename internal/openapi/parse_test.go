package openapi

import (
	"strings"
	"testing"
)

func TestParseRejectsIncompleteDocuments(t *testing.T) {
	for name, tt := range map[string]struct {
		spec string
		want string
	}{
		"empty":      {``, "decode OpenAPI document"},
		"not json":   {`{`, "decode OpenAPI document"},
		"no version": {`{"info": {"title": "T"}, "paths": {"/x": {}}}`, "openapi version"},
		"no title":   {`{"openapi": "3.1.0", "info": {}, "paths": {"/x": {}}}`, "info.title"},
		"no paths":   {`{"openapi": "3.1.0", "info": {"title": "T"}}`, "no paths"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Parse([]byte(tt.spec)); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("got %v, want an error containing %q", err, tt.want)
			}
		})
	}
}

func TestParseYAMLWithNumericStatusAndMergeKeys(t *testing.T) {
	const yamlSpec = `
openapi: 3.1.0
info:
  title: YAML API
  version: 1.2.3
x-defaults: &default_resp
  description: Default response
paths:
  /items:
    get:
      summary: List items
      responses:
        200:
          description: OK
          content:
            application/json:
              schema: true
        404:
          <<: *default_resp
`
	doc, err := Parse([]byte(yamlSpec))
	if err != nil {
		t.Fatalf("Parse YAML failed: %v", err)
	}
	if doc.Info.Title != "YAML API" || doc.OpenAPI != "3.1.0" {
		t.Fatalf("unexpected doc metadata: %+v", doc.Info)
	}
	pi := doc.Paths["/items"]
	op, err := pi.DecodeOperation("get", "/items")
	if err != nil {
		t.Fatalf("DecodeOperation failed: %v", err)
	}
	if op.Responses["200"].Description != "OK" {
		t.Fatalf("expected 200 OK, got %+v", op.Responses["200"])
	}
	s := op.Responses["200"].Content["application/json"].Schema
	if s == nil || s.Boolean == nil || !*s.Boolean {
		t.Fatalf("expected boolean true schema, got %+v", s)
	}
	if op.Responses["404"].Description != "Default response" {
		t.Fatalf("expected merged 404 description, got %+v", op.Responses["404"])
	}
}

func TestParseWebhooksOnlyDocument(t *testing.T) {
	const spec = `{
  "openapi": "3.1.0",
  "info": {"title": "Webhook API", "version": "1.0.0"},
  "webhooks": {
    "newPet": {
      "post": {
        "summary": "New pet event",
        "responses": {"200": {"description": "OK"}}
      }
    }
  }
}`
	doc, err := Parse([]byte(spec))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if len(doc.Webhooks) != 1 {
		t.Fatalf("expected 1 webhook, got %d", len(doc.Webhooks))
	}
}

func TestRefResolutionAndCycles(t *testing.T) {
	const spec = `{
  "openapi": "3.1.0",
  "info": {"title": "Ref API"},
  "paths": {
    "/a": {
      "$ref": "#/components/pathItems/SharedPath"
    }
  },
  "components": {
    "pathItems": {
      "SharedPath": {
        "get": {
          "parameters": [{"$ref": "#/components/parameters/CyclicA"}],
          "responses": {"200": {"$ref": "#/components/responses/OkResp"}}
        }
      }
    },
    "parameters": {
      "CyclicA": {"$ref": "#/components/parameters/CyclicB"},
      "CyclicB": {"$ref": "#/components/parameters/CyclicA"}
    },
    "responses": {
      "OkResp": {"description": "All good"}
    }
  }
}`
	doc, err := Parse([]byte(spec))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	pi := doc.ResolvePathItem(doc.Paths["/a"])
	op, err := pi.DecodeOperation("get", "/a")
	if err != nil {
		t.Fatalf("DecodeOperation failed: %v", err)
	}
	// Cyclic ref must terminate without hanging.
	_ = doc.ResolveParameter(op.Parameters[0])

	resp := doc.ResolveResponse(op.Responses["200"])
	if resp.Description != "All good" {
		t.Fatalf("expected resolved response description, got %q", resp.Description)
	}

	if name, ok := SchemaRefName("#/components/schemas/Foo~1Bar~0Baz"); !ok || name != "Foo/Bar~Baz" {
		t.Fatalf("unexpected unescaped ref name: %q, %v", name, ok)
	}
}
