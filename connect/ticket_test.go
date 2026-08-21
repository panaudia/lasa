package connect

import (
	"crypto/ed25519"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/panaudia/lasa/wire"
)

func testKeys(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	return pub, priv
}

func testClock() time.Time { return time.Unix(1_800_000_000, 0) }

func baseClaims() MintClaims {
	return MintClaims{
		Issuer:   "issuer-1",
		SpaceID:  "space1",
		ClientID: "c1",
		Roles:    []string{"moderator"},
		Permissions: Permissions{
			Set:  []string{"app.game."},
			Read: []string{"app."},
		},
		Entities: []Entity{{ID: "e1", Name: "Alice"}},
		IssuedAt: testClock().Add(-time.Hour),
	}
}

func verifier(pub ed25519.PublicKey) *Verifier {
	return &Verifier{SpaceID: "space1", Keys: []ed25519.PublicKey{pub}, Now: testClock}
}

func TestTicketMintVerifyRoundTrip(t *testing.T) {
	pub, priv := testKeys(t)
	raw, err := Mint(priv, baseClaims())
	if err != nil {
		t.Fatal(err)
	}
	tk, rej := verifier(pub).Verify(raw)
	if rej != nil {
		t.Fatal(rej)
	}
	if tk.ClientID != "c1" || tk.Issuer != "issuer-1" {
		t.Errorf("ticket = %+v", tk)
	}
	if len(tk.Roles) != 1 || tk.Roles[0] != "moderator" {
		t.Errorf("roles = %v", tk.Roles)
	}
	if len(tk.Entities) != 1 || tk.Entities[0].ID != "e1" {
		t.Errorf("entities = %+v", tk.Entities)
	}
	if tk.ExpiresAt != nil {
		t.Error("no exp minted; ticket must never expire")
	}
	if !tk.Permissions.MaySet("app.game.score") || tk.Permissions.MaySet("app.gamex") {
		t.Error("set grant matching wrong")
	}
	if !tk.Permissions.MayRead("app") || tk.Permissions.MayClear("app.game.score") {
		t.Error("read/clear grant matching wrong")
	}
}

func TestTicketVerifyRejects(t *testing.T) {
	pub, priv := testKeys(t)
	otherPub, otherPriv := testKeys(t)
	_ = otherPub

	expired := baseClaims()
	past := testClock().Add(-time.Minute)
	expired.ExpiresAt = &past

	future := baseClaims()
	nbf := testClock().Add(time.Hour)
	future.NotBefore = &nbf

	wrongSpace := baseClaims()
	wrongSpace.SpaceID = "space2"

	mint := func(priv ed25519.PrivateKey, c MintClaims) string {
		raw, err := Mint(priv, c)
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}

	cases := []struct {
		name string
		raw  string
	}{
		{"wrong key", mint(otherPriv, baseClaims())},
		{"expired", mint(priv, expired)},
		{"not yet valid", mint(priv, future)},
		{"wrong audience", mint(priv, wrongSpace)},
		{"garbage", "not.a.jwt"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, rej := verifier(pub).Verify(c.raw)
			if rej == nil {
				t.Fatal("want rejection")
			}
			if rej.Code != wire.ErrCodeInvalidTicket {
				t.Errorf("code = %#x, want ErrCodeInvalidTicket", rej.Code)
			}
		})
	}
}

// A valid signature over claims that violate the ticket schema must still
// be rejected: the schema is normative for the claims object.
func TestTicketVerifySchemaEnforced(t *testing.T) {
	pub, priv := testKeys(t)

	sign := func(claims jwt.MapClaims) string {
		raw, err := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims).SignedString(priv)
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	now := float64(testClock().Add(-time.Hour).Unix())

	cases := []struct {
		name   string
		claims jwt.MapClaims
	}{
		{"missing panaudia", jwt.MapClaims{"iss": "i", "iat": now, "aud": "space1"}},
		{"missing client_id", jwt.MapClaims{"iss": "i", "iat": now, "aud": "space1", "panaudia": map[string]any{}}},
		// The claims object is a closed set (§4.3): registered JWT
		// claims outside the profile — jti, sub, azp — reject.
		{"registered claim jti", jwt.MapClaims{"iss": "i", "iat": now, "aud": "space1", "jti": "t1",
			"panaudia": map[string]any{"client_id": "c1"}}},
		{"registered claim sub", jwt.MapClaims{"iss": "i", "iat": now, "aud": "space1", "sub": "someone",
			"panaudia": map[string]any{"client_id": "c1"}}},
		// Role names are grammar-checked only; vocabulary is profile
		// policy (base.ValidRoles), enforced at admission, not here.
		{"role name violates grammar", jwt.MapClaims{"iss": "i", "iat": now, "aud": "space1",
			"panaudia": map[string]any{"client_id": "c1", "roles": []string{"EMPEROR"}}}},
		{"bad permission prefix", jwt.MapClaims{"iss": "i", "iat": now, "aud": "space1",
			"panaudia": map[string]any{"client_id": "c1", "permissions": map[string]any{"read": []string{"BAD"}}}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, rej := verifier(pub).Verify(sign(c.claims))
			if rej == nil {
				t.Fatal("want rejection")
			}
			if rej.Code != wire.ErrCodeInvalidTicket {
				t.Errorf("code = %#x, want ErrCodeInvalidTicket", rej.Code)
			}
		})
	}

	// iat is optional (§4.3): a ticket without it verifies.
	t.Run("iat optional", func(t *testing.T) {
		claims := jwt.MapClaims{"iss": "i", "aud": "space1",
			"panaudia": map[string]any{"client_id": "c1"}}
		if _, rej := verifier(pub).Verify(sign(claims)); rej != nil {
			t.Fatalf("ticket without iat must verify: %v", rej.Reason)
		}
	})
	// ...but enforced when present: a future-dated iat rejects.
	t.Run("future iat rejects", func(t *testing.T) {
		future := float64(testClock().Add(time.Hour).Unix())
		claims := jwt.MapClaims{"iss": "i", "iat": future, "aud": "space1",
			"panaudia": map[string]any{"client_id": "c1"}}
		if _, rej := verifier(pub).Verify(sign(claims)); rej == nil {
			t.Fatal("future-dated iat must reject")
		}
	})
}

// Non-EdDSA algorithms are rejected outright, including alg=none.
func TestTicketAlgorithmAllowlist(t *testing.T) {
	pub, _ := testKeys(t)

	hs, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"iss": "i", "iat": 1, "aud": "space1",
		"panaudia": map[string]any{"client_id": "c1"},
	}).SignedString([]byte("secret"))
	if err != nil {
		t.Fatal(err)
	}
	none, err := jwt.NewWithClaims(jwt.SigningMethodNone, jwt.MapClaims{
		"iss": "i", "iat": 1, "aud": "space1",
		"panaudia": map[string]any{"client_id": "c1"},
	}).SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatal(err)
	}
	for name, raw := range map[string]string{"HS256": hs, "none": none} {
		if _, rej := verifier(pub).Verify(raw); rej == nil {
			t.Errorf("%s ticket must be rejected", name)
		}
	}
}

func TestTicketMultipleIssuerKeys(t *testing.T) {
	pub1, _ := testKeys(t)
	pub2, priv2 := testKeys(t)
	raw, err := Mint(priv2, baseClaims())
	if err != nil {
		t.Fatal(err)
	}
	v := &Verifier{SpaceID: "space1", Keys: []ed25519.PublicKey{pub1, pub2}, Now: testClock}
	if _, rej := v.Verify(raw); rej != nil {
		t.Fatalf("second configured key must verify: %v", rej)
	}
}
