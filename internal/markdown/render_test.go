package markdown

import (
	"regexp"
	"strings"
	"testing"

	"github.com/oxio/openapi2md/internal/openapi"
)

const testOpenAPI = `{
  "openapi": "3.1.0",
  "info": {"title": "Demo API", "version": "2.0.0", "description": "Prices are scaled integers."},
  "tags": [
    {"name": "Things", "description": "All the things."}
  ],
  "paths": {
    "/things/{id}": {
      "get": {
        "operationId": "getThing",
        "summary": "Get a thing",
        "description": "Returns one thing.",
        "tags": ["Things"],
        "parameters": [
          {"name": "id", "in": "path", "required": true, "description": "Thing ID.", "schema": {"type": "string", "examples": ["abc"]}}
        ],
        "responses": {
          "default": {"description": "Error"},
          "404": {"description": "Not Found", "content": {"application/problem+json": {"schema": {"$ref": "#/components/schemas/Problem"}}}},
          "200": {"description": "OK", "content": {"application/json": {"schema": {"$ref": "#/components/schemas/Thing"}}}}
        }
      },
      "delete": {
        "operationId": "deleteThing",
        "summary": "Delete a thing",
        "tags": ["Things"],
        "responses": {"204": {"description": "No Content"}}
      }
    },
    "/things": {
      "post": {
        "operationId": "createThing",
        "summary": "Create a thing",
        "tags": ["Things"],
        "requestBody": {"required": true, "content": {"application/json": {"schema": {"$ref": "#/components/schemas/Thing"}}}},
        "responses": {"201": {"description": "Created"}}
      }
    },
    "/health": {
      "get": {"operationId": "health", "summary": "Health", "responses": {"200": {"description": "OK"}}}
    }
  },
  "components": {
    "schemas": {
      "Thing": {
        "type": "object",
        "description": "A thing.",
        "required": ["id"],
        "properties": {
          "id": {"type": "string", "description": "Thing ID."},
          "price": {"type": "integer", "format": "int64", "description": "Scaled price.", "examples": [299900]},
          "tags": {"type": ["array", "null"], "items": {"type": "string"}, "description": "Labels."},
          "kind": {"type": "string", "enum": ["big", "small"], "default": "big", "description": "Size."},
          "parts": {"type": "array", "items": {"$ref": "#/components/schemas/Part"}}
        }
      },
      "Part": {"type": "object", "properties": {"name": {"type": "string"}}},
      "Problem": {"type": "object", "properties": {"detail": {"type": "string"}}},
      "Label": {"type": "string", "description": "A label."}
    }
  }
}`

func mustRender(t *testing.T, spec string, opts Options) string {
	t.Helper()
	doc, err := openapi.Parse([]byte(spec))
	if err != nil {
		t.Fatal(err)
	}
	got, err := Render(doc, opts)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func assertValidAnchors(t *testing.T, md string) {
	t.Helper()
	headings := make(map[string]int)
	inCode := false
	headingRe := regexp.MustCompile(`^#+\s+(.*)$`)
	for _, line := range strings.Split(md, "\n") {
		if strings.HasPrefix(line, "```") {
			inCode = !inCode
		}
		if inCode {
			continue
		}
		if m := headingRe.FindStringSubmatch(line); m != nil {
			headings[Anchor(m[1])]++
		}
	}
	for name, count := range headings {
		if count > 1 {
			t.Errorf("anchor #%s is used by %d headings", name, count)
		}
	}
	linkRe := regexp.MustCompile(`\]\(#([^)]+)\)`)
	for _, m := range linkRe.FindAllStringSubmatch(md, -1) {
		if headings[m[1]] == 0 {
			t.Errorf("link to #%s has no matching heading", m[1])
		}
	}
}

func TestRenderBasicSpec(t *testing.T) {
	opts := Options{
		SpecURL: "/openapi.json",
	}
	got := mustRender(t, testOpenAPI, opts)

	for _, want := range []string{
		DefaultHeader,
		"# Demo API",
		"Version 2.0.0, OpenAPI 3.1.0.",
		"Prices are scaled integers.",
		"`/openapi.json`",
		"## Things",
		"All the things.",
		"### GET /things/{id}",
		"**Get a thing** (`getThing`)",
		"| `id` | path | string | yes | Thing ID. Example: `\"abc\"`. |",
		"| 200 | OK | [Thing](#thing) (`application/json`) |",
		"| 404 | Not Found | [Problem](#problem) (`application/problem+json`) |",
		"| 204 | No Content | none |",
		"**Request body** (required)",
		"- `application/json`: [Thing](#thing)",
		"## Other",
		"### GET /health",
		"### Thing",
		"| `id` | string | yes | Thing ID. |",
		"| `price` | integer (int64) | no | Scaled price. Example: `299900`. |",
		"| `tags` | array of string, nullable | no | Labels. |",
		"| `kind` | string (one of `big`, `small`) | no | Size. Default: `\"big\"`. |",
		"| `parts` | array of [Part](#part) | no |  |",
		"**Type:** string",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered docs do not contain %q:\n%s", want, got)
		}
	}

	// Operations are grouped by tag and sorted by path, then method inside a group;
	// untagged ones last; statuses are sorted numerically with "default" last.
	order := []string{"### POST /things", "### GET /things/{id}", "### DELETE /things/{id}", "### GET /health"}
	last := -1
	for _, heading := range order {
		i := strings.Index(got, heading)
		if i < last {
			t.Errorf("%q is out of order", heading)
		}
		last = i
	}
	if strings.Index(got, "| 200 |") > strings.Index(got, "| 404 |") || strings.Index(got, "| 404 |") > strings.Index(got, "| default |") {
		t.Errorf("responses are not sorted:\n%s", got)
	}

	assertValidAnchors(t, got)

	if again := mustRender(t, testOpenAPI, opts); again != got {
		t.Error("Render is not deterministic")
	}
}

func TestRenderRejectsUndecodableOperation(t *testing.T) {
	doc, err := openapi.Parse([]byte(`{"openapi": "3.1.0", "info": {"title": "T"}, "paths": {"/x": {"get": "nope"}}}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Render(doc, Options{}); err == nil || !strings.Contains(err.Error(), "GET /x") {
		t.Fatalf("got %v, want an error naming the operation", err)
	}
}

func TestRenderCompositionAnyAndFullOpenAPI31(t *testing.T) {
	const fullSpec = `
openapi: 3.1.0
info:
  title: Full Feature API
  version: 3.0.0
  summary: Exercises oneOf, anyOf, allOf, not, any, never, webhooks, auth, and refs.
  contact:
    name: API Team
    email: api@example.com
  license:
    name: MIT
    url: https://opensource.org/licenses/MIT
servers:
  - url: https://{env}.api.example.com/v1
    description: Regional API
    variables:
      env:
        default: prod
        enum: [prod, staging]
        description: Deployment environment
security:
  - bearerAuth: []
tags:
  - name: Pets
    description: Manage pets.
paths:
  /pets/{id}:
    parameters:
      - $ref: '#/components/parameters/PetId'
      - name: trace_id
        in: header
        schema:
          type: string
    get:
      operationId: getPet
      summary: Fetch a pet
      tags: [Pets]
      deprecated: true
      security: []
      parameters:
        - name: trace_id
          in: header
          description: Overridden trace header.
          schema:
            type: string
            minLength: 8
            maxLength: 64
      responses:
        200:
          description: Found
          headers:
            X-Rate-Limit:
              description: Remaining requests
              required: true
              schema:
                type: integer
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/Pet'
        4XX:
          description: Client error
        default:
          $ref: '#/components/responses/ErrorResponse'
    post:
      operationId: updatePetMetadata
      summary: Update arbitrary metadata
      tags: [Pets]
      requestBody:
        $ref: '#/components/requestBodies/MetadataBody'
      responses:
        204:
          description: Updated
webhooks:
  petAdopted:
    post:
      operationId: onPetAdopted
      summary: Fired when a pet is adopted
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/Pet'
      responses:
        200:
          description: Acknowledged
components:
  securitySchemes:
    bearerAuth:
      type: http
      scheme: bearer
      bearerFormat: JWT
      description: Bearer JWT token
    apiKeyAuth:
      type: apiKey
      name: X-API-Key
      in: header
  parameters:
    PetId:
      name: id
      in: path
      required: true
      description: Pet identifier.
      schema:
        type: string
        format: uuid
  requestBodies:
    MetadataBody:
      required: true
      description: Arbitrary metadata payload.
      content:
        application/json:
          schema: {}
  responses:
    ErrorResponse:
      description: Generic error
      content:
        application/json:
          schema:
            type: object
            properties:
              message:
                type: string
  schemas:
    Pets:
      type: array
      description: Intentionally collides with the Pets tag heading to test anchor disambiguation.
      items:
        $ref: '#/components/schemas/Pet'
    Pet:
      description: Polymorphic pet union.
      oneOf:
        - $ref: '#/components/schemas/Cat'
        - $ref: '#/components/schemas/Dog'
      discriminator:
        propertyName: petType
        mapping:
          cat: '#/components/schemas/Cat'
          dog: '#/components/schemas/Dog'
    Cat:
      allOf:
        - $ref: '#/components/schemas/BaseAnimal'
        - type: object
          required: [lives]
          properties:
            petType:
              const: cat
            lives:
              type: integer
              minimum: 1
              maximum: 9
    Dog:
      allOf:
        - $ref: '#/components/schemas/BaseAnimal'
        - type: object
          properties:
            petType:
              const: dog
            barkVolume:
              type: number
              exclusiveMinimum: 0
    BaseAnimal:
      type: object
      required: [name]
      properties:
        name:
          type: string
          minLength: 1
        nickname:
          anyOf:
            - type: string
            - type: "null"
        payload: {}
        strictlyNothing: false
        anythingTrue: true
        flexible:
          anyOf:
            - type: string
            - type: integer
        multiType:
          type: [string, integer, "null"]
        coordinates:
          type: array
          prefixItems:
            - type: number
            - type: number
          items: false
        attributes:
          type: object
          additionalProperties:
            type: string
        notNull:
          not:
            type: "null"
    AnyValue: {}
    NeverValue: false
    FlexibleUnion:
      anyOf:
        - $ref: '#/components/schemas/Cat'
        - type: string
`

	got := mustRender(t, fullSpec, Options{TOC: true})

	for _, want := range []string{
		"## Table of Contents",
		"## Servers",
		"`https://{env}.api.example.com/v1`",
		"`env`: default `prod` (one of `prod`, `staging`)",
		"## Authentication",
		"| `bearerAuth` | http | scheme `bearer` (JWT). Bearer JWT token |",
		"| `apiKeyAuth` | apiKey | `X-API-Key` in header |",
		"**Default security:** `bearerAuth`",
		"*(Deprecated)*",
		"**Security:** none (public)",
		"| `id` | path | string (uuid) | yes | Pet identifier. |",
		"Overridden trace header. Constraints: min length 8, max length 64.",
		"**Response headers**",
		"| 200 | `X-Rate-Limit` | integer | yes | Remaining requests. |",
		"- `application/json`: any",
		"## Webhooks",
		"### POST petAdopted",
		"### Pets (Schema)",
		"**One of:** [Cat](#cat) | [Dog](#dog)",
		"**Discriminator:** `petType`",
		"| `cat` | [Cat](#cat) |",
		"| `dog` | [Dog](#dog) |",
		"**All of:** [BaseAnimal](#baseanimal)",
		"| `lives` | integer | yes | Constraints: >= 1, <= 9. |",
		"| `petType` | const `\"cat\"` | no |  |",
		"| `barkVolume` | number | no | Constraints: > 0. |",
		"| `nickname` | string, nullable | no |  |",
		"| `payload` | any | no |  |",
		"| `strictlyNothing` | never | no |  |",
		"| `anythingTrue` | any | no |  |",
		"| `flexible` | any of: string \\| integer | no |  |",
		"| `multiType` | string \\| integer, nullable | no |  |",
		"| `coordinates` | tuple of (number, number) | no |  |",
		"| `attributes` | map of string to string | no |  |",
		"| `notNull` | not null | no |  |",
		"### AnyValue\n\n**Type:** any",
		"### NeverValue\n\n**Type:** never",
		"**Any of:** [Cat](#cat) | string",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered full spec missing %q:\n%s", want, got)
		}
	}

	assertValidAnchors(t, got)
}

func TestRenderOptions(t *testing.T) {
	got := mustRender(t, testOpenAPI, Options{
		NoHeader:  true,
		Title:     "Custom Title",
		NoSchemas: true,
	})
	if strings.Contains(got, "<!--") {
		t.Error("expected header comment to be omitted")
	}
	if !strings.HasPrefix(got, "# Custom Title\n") {
		t.Errorf("expected custom title at top, got:\n%s", got[:min(80, len(got))])
	}
	if strings.Contains(got, "## Schemas") {
		t.Error("expected Schemas section to be omitted")
	}
}
