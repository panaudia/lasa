// Create and sign a LASA ticket with Go.
// Format: schemata/ticket.schema.json (lasa-core.md §4.3).
// Run with: go run .
package main

import (
	"crypto/ed25519"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"time"

	jwt "github.com/golang-jwt/jwt/v5"
)

func main() {
	// Load private key from PEM
	keyData, err := os.ReadFile("private.pem")
	if err != nil {
		panic(err)
	}
	block, _ := pem.Decode(keyData)
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		panic(err)
	}
	privateKey := parsed.(ed25519.PrivateKey)

	now := time.Now().Unix()

	// MapClaims rather than RegisteredClaims: the schema requires aud as a
	// plain string, and golang-jwt marshals a RegisteredClaims audience as
	// an array by default.
	claims := jwt.MapClaims{
		"iss": "my-app",
		// aud MUST equal the space id: [a-z0-9-] only, no underscores.
		"aud": "df8c7a85-0702-45e9-a626-a1c147eafce9",
		"iat": now,
		// exp is the only bound on ticket reuse — set it deliberately.
		"exp": now + 86400,
		"panaudia": map[string]any{
			// Identity: the config's client_id must equal this claim.
			"client_id": "6f1c2d3e-8a4b-4c5d-9e0f-1a2b3c4d5e6f",
			"roles":     []string{"presenter"},
			// Explicit state-access grants (key prefixes), union'd with roles.
			"permissions": map[string]any{
				"set":   []string{"myapp."},
				"clear": []string{"myapp."},
				"read":  []string{"myapp."},
			},
			// Signed entity definitions, instantiated by the config's setups.
			"entities": []any{
				map[string]any{
					"id":       "a1b2c3d4-0000-0000-0000-000000000001",
					"name":     "Paul",
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

	ticket, err := token.SignedString(privateKey)
	if err != nil {
		panic(err)
	}
	fmt.Println(ticket)
}
