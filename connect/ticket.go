package connect

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/panaudia/lasa/ident"
)

// Permissions are a ticket's explicit state-access grants: three arrays of
// key prefixes matched exact-or-dot-extended (lasa-core.md §4.3, §6.4).
type Permissions struct {
	Set   []string `json:"set,omitempty"`
	Clear []string `json:"clear,omitempty"`
	Read  []string `json:"read,omitempty"`
}

// MaySet reports whether key falls under any set grant.
func (p Permissions) MaySet(key string) bool { return anyScopeMatches(p.Set, key) }

// MayClear reports whether key falls under any clear grant.
func (p Permissions) MayClear(key string) bool { return anyScopeMatches(p.Clear, key) }

// MayRead reports whether key falls under any read grant.
func (p Permissions) MayRead(key string) bool { return anyScopeMatches(p.Read, key) }

func anyScopeMatches(scopes []string, key string) bool {
	for _, s := range scopes {
		if ident.ScopeMatches(s, key) {
			return true
		}
	}
	return false
}

// Ticket is a verified LASA ticket: the JWT's panaudia claim plus the
// registered claims the server consults after admission.
type Ticket struct {
	ClientID    string
	Roles       []string
	Permissions Permissions
	Entities    []Entity

	Issuer    string
	ExpiresAt *time.Time // nil: never expires; the only bound on reuse
}

// Verifier verifies tickets for one Space (lasa-core.md §4.3): signature
// against the configured issuer keys, aud equal to the space id, exp/nbf
// when present. Only EdDSA (Ed25519) signatures are accepted.
type Verifier struct {
	SpaceID string
	Keys    []ed25519.PublicKey

	// Now is the clock for exp/nbf checks; nil means time.Now.
	Now func() time.Time
}

type ticketClaims struct {
	jwt.RegisteredClaims
	Panaudia panaudiaClaim `json:"panaudia"`
}

type panaudiaClaim struct {
	ClientID    string      `json:"client_id"`
	Roles       []string    `json:"roles,omitempty"`
	Permissions Permissions `json:"permissions,omitempty"`
	Entities    []Entity    `json:"entities,omitempty"`
}

// Verify checks raw and returns the verified ticket, or a Rejection with
// code ErrCodeInvalidTicket.
func (v *Verifier) Verify(raw string) (*Ticket, *Rejection) {
	now := time.Now
	if v.Now != nil {
		now = v.Now
	}
	keys := make([]jwt.VerificationKey, len(v.Keys))
	for i, k := range v.Keys {
		keys[i] = k
	}
	var claims ticketClaims
	_, err := jwt.NewParser(
		jwt.WithValidMethods([]string{"EdDSA"}),
		jwt.WithTimeFunc(now),
		jwt.WithAudience(v.SpaceID),
	).ParseWithClaims(raw, &claims, func(*jwt.Token) (any, error) {
		return jwt.VerificationKeySet{Keys: keys}, nil
	})
	if err != nil {
		return nil, rejectTicket("ticket: %v", err)
	}

	// The signature is good; now hold the claims object to the normative
	// ticket schema (required fields, identifier grammars, role enum,
	// permission prefix shapes).
	if rej := v.validateClaimsSchema(raw); rej != nil {
		return nil, rej
	}

	t := &Ticket{
		ClientID:    claims.Panaudia.ClientID,
		Roles:       claims.Panaudia.Roles,
		Permissions: claims.Panaudia.Permissions,
		Entities:    claims.Panaudia.Entities,
		Issuer:      claims.Issuer,
	}
	if claims.ExpiresAt != nil {
		exp := claims.ExpiresAt.Time
		t.ExpiresAt = &exp
	}
	return t, nil
}

func (v *Verifier) validateClaimsSchema(raw string) *Rejection {
	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		return rejectTicket("ticket: not a JWT")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return rejectTicket("ticket: %v", err)
	}
	_, ticketSchema, err := schemas()
	if err != nil {
		return rejectTicket("internal: schemas unavailable: %v", err)
	}
	if err := validateAgainst(ticketSchema, payload); err != nil {
		return rejectTicket("ticket claims: %v", err)
	}
	return nil
}

// MintClaims describes a ticket to mint. SpaceID becomes aud; a nil
// ExpiresAt mints a ticket that never expires — set it deliberately, as it
// is the only bound on reuse (lasa-core.md §4.3).
type MintClaims struct {
	Issuer      string
	SpaceID     string
	ClientID    string
	Roles       []string
	Permissions Permissions
	Entities    []Entity
	IssuedAt    time.Time
	ExpiresAt   *time.Time
	NotBefore   *time.Time
}

// Mint signs a ticket with an Ed25519 issuer key. Provided for issuer
// tooling and tests; verification does not depend on it.
func Mint(key ed25519.PrivateKey, c MintClaims) (string, error) {
	claims := ticketClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:   c.Issuer,
			Audience: jwt.ClaimStrings{c.SpaceID},
			IssuedAt: jwt.NewNumericDate(c.IssuedAt),
		},
		Panaudia: panaudiaClaim{
			ClientID:    c.ClientID,
			Roles:       c.Roles,
			Permissions: c.Permissions,
			Entities:    c.Entities,
		},
	}
	if c.ExpiresAt != nil {
		claims.ExpiresAt = jwt.NewNumericDate(*c.ExpiresAt)
	}
	if c.NotBefore != nil {
		claims.NotBefore = jwt.NewNumericDate(*c.NotBefore)
	}
	return jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims).SignedString(key)
}

// MarshalJSON flattens the registered claims and the panaudia claim into
// one object with aud as a plain string — jwt.RegisteredClaims would
// marshal aud as an array, which ticket.schema.json forbids.
func (c ticketClaims) MarshalJSON() ([]byte, error) {
	m := map[string]any{
		"iss":      c.Issuer,
		"iat":      c.IssuedAt,
		"panaudia": c.Panaudia,
	}
	if len(c.Audience) == 1 {
		m["aud"] = c.Audience[0]
	} else {
		m["aud"] = c.Audience
	}
	if c.ExpiresAt != nil {
		m["exp"] = c.ExpiresAt
	}
	if c.NotBefore != nil {
		m["nbf"] = c.NotBefore
	}
	return json.Marshal(m)
}
