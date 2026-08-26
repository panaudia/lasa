package connect

import (
	"crypto/ed25519"
	"crypto/rand"
	"strings"
	"testing"
	"time"
)

const goodClaims = `{
  "iss": "test",
  "aud": "space-1",
  "exp": 4102444800,
  "panaudia": {
    "client_id": "client-1",
    "roles": ["presenter"],
    "permissions": {"set": ["app."], "read": ["app."]},
    "entities": [{"id": "ent-1", "name": "One", "heard-in": ["main"]}]
  }
}`

func TestParseClaimsRoundTrip(t *testing.T) {
	c, err := ParseClaims([]byte(goodClaims))
	if err != nil {
		t.Fatal(err)
	}
	if c.SpaceID != "space-1" || c.ClientID != "client-1" || c.Issuer != "test" {
		t.Fatalf("decoded %+v", c)
	}
	if c.ExpiresAt == nil || c.ExpiresAt.Unix() != 4102444800 || c.NotBefore != nil {
		t.Fatalf("times %v %v", c.ExpiresAt, c.NotBefore)
	}
	if len(c.Entities) != 1 || c.Entities[0].HeardIn[0] != "main" {
		t.Fatalf("entities %+v", c.Entities)
	}
	out, err := FormatClaims(c)
	if err != nil {
		t.Fatal(err)
	}
	again, err := ParseClaims(out)
	if err != nil {
		t.Fatalf("re-parse: %v\n%s", err, out)
	}
	if again.SpaceID != c.SpaceID || again.ClientID != c.ClientID || again.ExpiresAt.Unix() != c.ExpiresAt.Unix() {
		t.Fatalf("round trip lost data: %+v", again)
	}
	if strings.Contains(string(out), `"iat"`) {
		t.Fatalf("FormatClaims must not write iat:\n%s", out)
	}
}

func TestParseClaimsRejects(t *testing.T) {
	cases := map[string]string{
		"array aud":      `{"iss":"t","aud":["space-1"],"panaudia":{"client_id":"c"}}`,
		"iat present":    `{"iss":"t","aud":"space-1","iat":1,"panaudia":{"client_id":"c"}}`,
		"unknown key":    `{"iss":"t","aud":"space-1","jti":"x","panaudia":{"client_id":"c"}}`,
		"no client_id":   `{"iss":"t","aud":"space-1","panaudia":{}}`,
		"no aud":         `{"iss":"t","panaudia":{"client_id":"c"}}`,
		"no iss":         `{"aud":"space-1","panaudia":{"client_id":"c"}}`,
		"trailing data":  `{"iss":"t","aud":"space-1","panaudia":{"client_id":"c"}} {}`,
		"unknown nested": `{"iss":"t","aud":"space-1","panaudia":{"client_id":"c","extra":1}}`,
	}
	for name, in := range cases {
		if _, err := ParseClaims([]byte(in)); err == nil {
			t.Errorf("%s: accepted %s", name, in)
		}
	}
}

// A claims file minted with Mint verifies with Verifier: the file shape
// and the token shape agree.
func TestParseClaimsMintVerify(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	c, err := ParseClaims([]byte(goodClaims))
	if err != nil {
		t.Fatal(err)
	}
	c.IssuedAt = time.Now()
	tok, err := Mint(priv, c)
	if err != nil {
		t.Fatal(err)
	}
	v := &Verifier{SpaceID: "space-1", Keys: []ed25519.PublicKey{pub}}
	tk, rej := v.Verify(tok)
	if rej != nil {
		t.Fatalf("verify: %v", rej)
	}
	if tk.ClientID != "client-1" || tk.Roles[0] != "presenter" || !tk.Permissions.MaySet("app.x") {
		t.Fatalf("ticket %+v", tk)
	}
}
