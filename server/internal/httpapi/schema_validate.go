package httpapi

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/santhosh-tekuri/jsonschema/v5"
)

var pushRequestSchema *jsonschema.Schema

func init() {
	const raw = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "Sentra PushRequest v1",
  "type": "object",
  "additionalProperties": false,
  "required": ["v", "project", "machine", "commit", "files"],
  "properties": {
    "v": {"type": "integer", "const": 1},
    "project": {
      "type": "object",
      "additionalProperties": false,
      "properties": {
        "id": {"type": "string", "format": "uuid"},
        "root": {"type": "string", "minLength": 1, "maxLength": 300}
      },
      "oneOf": [
        {"required": ["id"], "not": {"required": ["root"]}},
        {"required": ["root"], "not": {"required": ["id"]}}
      ]
    },
    "machine": {
      "type": "object",
      "additionalProperties": false,
      "required": ["id"],
      "properties": {
        "id": {"type": "string", "format": "uuid"},
        "name": {"type": "string", "minLength": 1, "maxLength": 255}
      }
    },
    "commit": {
      "type": "object",
      "additionalProperties": false,
      "required": ["client_id", "message"],
      "properties": {
        "client_id": {
          "type": "string",
          "description": "Client-generated UUID for idempotency. The server must treat (user_id, project, client_id) as unique and return the existing commit on retries.",
          "format": "uuid"
        },
        "message": {"type": "string", "minLength": 1, "maxLength": 500},
        "parent_client_id": {"type": "string", "format": "uuid"}
      }
    },
    "files": {
      "type": "array",
      "minItems": 1,
      "maxItems": 200,
      "items": {
        "type": "object",
        "additionalProperties": false,
		"required": ["path", "sha256", "size", "encrypted", "cipher"],
		"properties": {
		  "path": {
			"type": "string",
			"minLength": 1,
			"maxLength": 500,
			"pattern": "^(?:[A-Za-z0-9._-]+/)*\\.env(?:\\.[A-Za-z0-9._-]+)*$"
		  },
          "sha256": {
            "type": "string",
            "description": "SHA-256 (hex, lowercase) of the plaintext .env contents before encryption. Used for integrity and deduplication; the server cannot verify it without decrypting.",
            "pattern": "^[a-f0-9]{64}$"
          },
          "size": {"type": "integer", "minimum": 1, "maximum": 1048576},
          "encrypted": {"type": "boolean", "const": true},
		  "cipher": {"type": "string", "enum": ["ed25519+aes-256-gcm-v1", "age-v1", "sentra-v1"]},
		  "blob": {"type": "string", "minLength": 1, "maxLength": 8000000},
		  "storage": {
			"type": "object",
			"additionalProperties": false,
			"required": ["provider", "bucket", "key"],
			"properties": {
			  "provider": {"type": "string", "enum": ["s3"]},
			  "bucket": {"type": "string", "minLength": 1, "maxLength": 255},
			  "key": {"type": "string", "minLength": 1, "maxLength": 1024},
			  "endpoint": {"type": "string", "minLength": 1, "maxLength": 500},
			  "region": {"type": "string", "minLength": 1, "maxLength": 100}
			}
		  }
		}
		,
		"oneOf": [
		  {"required": ["blob"]},
		  {"required": ["storage"]}
		]
	  }
	}
  }
}`

	compiler := jsonschema.NewCompiler()
	compiler.AssertFormat = true
	compiler.Formats["uuid"] = func(v any) bool {
		s, ok := v.(string)
		if !ok {
			return false
		}
		_, err := uuid.Parse(strings.TrimSpace(s))
		return err == nil
	}

	if err := compiler.AddResource("sentra://push.v1.schema.json", strings.NewReader(raw)); err != nil {
		panic(err)
	}
	s, err := compiler.Compile("sentra://push.v1.schema.json")
	if err != nil {
		panic(err)
	}
	pushRequestSchema = s
}

func validateJSONAgainstPushSchema(body []byte) error {
	if pushRequestSchema == nil {
		return fmt.Errorf("push schema unavailable")
	}
	var v any
	if err := json.Unmarshal(body, &v); err != nil {
		return err
	}
	return pushRequestSchema.Validate(v)
}
