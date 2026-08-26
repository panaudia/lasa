package connect

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// A claims file is the ticket's JWT payload minus iat: iss, aud, optional
// exp/nbf (epoch seconds), and the panaudia claim, exactly as
// ticket.schema.json describes the signed token. It is the durable
// artefact issuer tooling keeps and re-mints from (plan/ticket-cli.md).
type claimsFile struct {
	Iss      *string         `json:"iss"`
	Aud      json.RawMessage `json:"aud"`
	Exp      *int64          `json:"exp,omitempty"`
	Nbf      *int64          `json:"nbf,omitempty"`
	Iat      *int64          `json:"iat,omitempty"`
	Panaudia *panaudiaClaim  `json:"panaudia"`
}

// ParseClaims decodes a claims file into MintClaims. Unknown keys, an
// array-valued aud and a present iat are errors: the file must be the
// payload the token will carry, and iat is stamped at mint time.
// IssuedAt is left zero for the caller to set.
func ParseClaims(data []byte) (MintClaims, error) {
	var f claimsFile
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&f); err != nil {
		return MintClaims{}, fmt.Errorf("claims: %w", err)
	}
	if dec.More() {
		return MintClaims{}, errors.New("claims: trailing data after the JSON object")
	}
	if f.Iss == nil {
		return MintClaims{}, errors.New("claims: iss is required")
	}
	if len(f.Aud) == 0 {
		return MintClaims{}, errors.New("claims: aud (the space id) is required")
	}
	var aud string
	if err := json.Unmarshal(f.Aud, &aud); err != nil {
		return MintClaims{}, errors.New("claims: aud must be a single string, the space id (not an array)")
	}
	if f.Iat != nil {
		return MintClaims{}, errors.New("claims: iat is stamped when the ticket is minted; remove it from the file")
	}
	if f.Panaudia == nil {
		return MintClaims{}, errors.New("claims: panaudia is required")
	}
	if f.Panaudia.ClientID == "" {
		return MintClaims{}, errors.New("claims: panaudia.client_id is required")
	}
	c := MintClaims{
		Issuer:      *f.Iss,
		SpaceID:     aud,
		ClientID:    f.Panaudia.ClientID,
		Roles:       f.Panaudia.Roles,
		Permissions: f.Panaudia.Permissions,
		Entities:    f.Panaudia.Entities,
	}
	if f.Exp != nil {
		t := time.Unix(*f.Exp, 0).UTC()
		c.ExpiresAt = &t
	}
	if f.Nbf != nil {
		t := time.Unix(*f.Nbf, 0).UTC()
		c.NotBefore = &t
	}
	return c, nil
}

// FormatClaims renders MintClaims as a claims file (indented JSON, no
// iat), the inverse of ParseClaims.
func FormatClaims(c MintClaims) ([]byte, error) {
	// A struct, not a map, so the file reads in schema order.
	f := struct {
		Iss      string        `json:"iss"`
		Aud      string        `json:"aud"`
		Exp      *int64        `json:"exp,omitempty"`
		Nbf      *int64        `json:"nbf,omitempty"`
		Panaudia panaudiaClaim `json:"panaudia"`
	}{
		Iss: c.Issuer,
		Aud: c.SpaceID,
		Panaudia: panaudiaClaim{
			ClientID:    c.ClientID,
			Roles:       c.Roles,
			Permissions: c.Permissions,
			Entities:    c.Entities,
		},
	}
	if c.ExpiresAt != nil {
		v := c.ExpiresAt.Unix()
		f.Exp = &v
	}
	if c.NotBefore != nil {
		v := c.NotBefore.Unix()
		f.Nbf = &v
	}
	return json.MarshalIndent(f, "", "  ")
}

// ValidateClaims holds a decoded JWT payload (or a claims file plus iat)
// to ticket.schema.json without checking any signature. It is what
// Verify applies after the signature check, exposed for tooling that
// wants the schema verdict on its own.
func ValidateClaims(payload []byte) error {
	_, ticketSchema, err := schemas()
	if err != nil {
		return fmt.Errorf("schemas unavailable: %w", err)
	}
	return validateAgainst(ticketSchema, payload)
}
