// Verify Go LASA ticket creation and signing.
// Run with: go run .
package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func main() {
	// --- Step 1: Generate keys ---
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		panic(err)
	}
	fmt.Println("Keys generated OK")

	// --- Step 2: Create and sign ticket ---
	spaceID := "df8c7a85-0702-45e9-a626-a1c147eafce9"
	clientID := "6f1c2d3e-8a4b-4c5d-9e0f-1a2b3c4d5e6f"
	entityID := "a1b2c3d4-0000-0000-0000-000000000001"
	now := time.Now().Unix()

	// MapClaims rather than RegisteredClaims: the schema requires aud as a
	// plain string, and golang-jwt marshals a RegisteredClaims audience as
	// an array by default.
	claims := jwt.MapClaims{
		"iss": "test-verify",
		"aud": spaceID,
		"iat": now,
		"exp": now + 3600,
		"panaudia": map[string]any{
			"client_id": clientID,
			"roles":     []string{"presenter"},
			"permissions": map[string]any{
				"set":   []string{"myapp."},
				"clear": []string{"myapp."},
				"read":  []string{"myapp."},
			},
			"entities": []any{
				map[string]any{
					"id":       entityID,
					"name":     "TestUser",
					"priority": true,
					"heard-in": []string{"main"},
					"hears":    []string{"main"},
					"attrs":    map[string]any{"colour": "00aaff"},
					"render":   map[string]any{"gain": 1.5, "attenuation": 2.0},
				},
			},
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)
	token.Header["typ"] = "JWT"
	token.Header["crv"] = "Ed25519"

	ticket, err := token.SignedString(priv)
	if err != nil {
		panic(fmt.Sprintf("SignedString failed: %v", err))
	}
	fmt.Printf("Ticket created OK (%d chars)\n", len(ticket))

	// --- Step 3: Verify ---
	parts := strings.Split(ticket, ".")
	if len(parts) != 3 {
		panic(fmt.Sprintf("Expected 3 parts, got %d", len(parts)))
	}

	// Decode header
	headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		panic(err)
	}
	var header map[string]interface{}
	json.Unmarshal(headerBytes, &header)

	assertEqual("typ", header["typ"], "JWT")
	assertEqual("alg", header["alg"], "EdDSA")
	assertEqual("crv", header["crv"], "Ed25519")
	fmt.Printf("Header OK: %v\n", header)

	// Decode payload
	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		panic(err)
	}
	var payload map[string]interface{}
	json.Unmarshal(payloadBytes, &payload)

	assertEqual("iss", payload["iss"], "test-verify")

	// aud must be a plain string — the schema forbids the array form
	aud, ok := payload["aud"].(string)
	if !ok {
		panic(fmt.Sprintf("aud must be a plain string, got %T", payload["aud"]))
	}
	assertEqual("aud", aud, spaceID)

	if _, ok := payload["iat"]; !ok {
		panic("missing iat")
	}
	if _, ok := payload["exp"]; !ok {
		panic("missing exp")
	}

	panaudia := payload["panaudia"].(map[string]interface{})
	assertEqual("client_id", panaudia["client_id"], clientID)
	roles := panaudia["roles"].([]interface{})
	assertEqual("roles[0]", roles[0], "presenter")
	permissions := panaudia["permissions"].(map[string]interface{})
	assertEqual("permissions.set[0]", permissions["set"].([]interface{})[0], "myapp.")
	assertEqual("permissions.clear[0]", permissions["clear"].([]interface{})[0], "myapp.")
	assertEqual("permissions.read[0]", permissions["read"].([]interface{})[0], "myapp.")
	entity := panaudia["entities"].([]interface{})[0].(map[string]interface{})
	assertEqual("entity.id", entity["id"], entityID)
	assertEqual("entity.name", entity["name"], "TestUser")
	assertEqual("entity.priority", entity["priority"], true)
	assertEqual("entity.heard-in[0]", entity["heard-in"].([]interface{})[0], "main")
	assertEqual("entity.hears[0]", entity["hears"].([]interface{})[0], "main")
	attrs := entity["attrs"].(map[string]interface{})
	assertEqual("entity.attrs.colour", attrs["colour"], "00aaff")
	render := entity["render"].(map[string]interface{})
	assertEqualFloat("entity.render.gain", render["gain"].(float64), 1.5)
	assertEqualFloat("entity.render.attenuation", render["attenuation"].(float64), 2.0)
	fmt.Println("Payload OK: all claims verified")

	// Verify signature with public key
	parsed, err := jwt.Parse(ticket, func(t *jwt.Token) (interface{}, error) {
		return pub, nil
	}, jwt.WithValidMethods([]string{"EdDSA"}), jwt.WithAudience(spaceID))
	if err != nil {
		panic(fmt.Sprintf("Signature verification failed: %v", err))
	}
	if !parsed.Valid {
		panic("Token not valid")
	}
	fmt.Println("Signature verified OK with public key")

	fmt.Println("\n=== GO: ALL TESTS PASSED ===")
}

func assertEqual(name string, got, want interface{}) {
	if got != want {
		panic(fmt.Sprintf("%s: got %v, want %v", name, got, want))
	}
}

func assertEqualFloat(name string, got, want float64) {
	if got != want {
		panic(fmt.Sprintf("%s: got %v, want %v", name, got, want))
	}
}
