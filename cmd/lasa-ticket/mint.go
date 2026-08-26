package main

import (
	"crypto/ed25519"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/panaudia/lasa/connect"
	"github.com/panaudia/lasa/profile/base"
)

// mint: claims file → connect.MintClaims → connect.Mint → connect.Verify.
// The self-verification uses the public half of the signing key and the
// file's aud as the space id, so the printed token is one the server's
// verifier accepts for that space, by construction.
func cmdMint(args []string) error {
	fs := flag.NewFlagSet("mint", flag.ContinueOnError)
	keyPath := fs.String("key", "", "issuer private key (PKCS#8 PEM, or - for stdin)")
	expDur := fs.String("exp", "", "expiry as a duration from now (90m, 24h, 7d); overrides exp in the file")
	nbfDur := fs.String("nbf", "", "not-before as a duration from now; overrides nbf in the file")
	iss := fs.String("iss", "", "override iss")
	out := fs.String("out", "", "write the token here instead of stdout")
	var allowRoles multiFlag
	fs.Var(&allowRoles, "allow-role", "accept a role outside the base-profile vocabulary (repeatable)")
	pos, err := parseArgs(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 1 {
		return usagef("usage: lasa-ticket mint --key private.pem CLAIMS.json [--exp DUR] [--nbf DUR] [--iss NAME] [--out FILE]")
	}
	if *keyPath == "" {
		return usagef("--key is required")
	}
	if *keyPath == "-" && pos[0] == "-" {
		return usagef("only one of --key and CLAIMS.json can be stdin")
	}
	keyData, err := readArg(*keyPath)
	if err != nil {
		return err
	}
	priv, err := parsePrivatePEM(keyData)
	if err != nil {
		return fmt.Errorf("--key: %v", err)
	}
	claimsData, err := readArg(pos[0])
	if err != nil {
		return err
	}
	c, err := connect.ParseClaims(claimsData)
	if err != nil {
		return err
	}
	now := time.Now().Truncate(time.Second)
	c.IssuedAt = now
	if *iss != "" {
		c.Issuer = *iss
	}
	if *expDur != "" {
		d, err := parseDuration(*expDur)
		if err != nil {
			return usagef("--exp: %v", err)
		}
		if d <= 0 {
			return usagef("--exp must be a positive duration")
		}
		t := now.Add(d)
		c.ExpiresAt = &t
	}
	if *nbfDur != "" {
		d, err := parseDuration(*nbfDur)
		if err != nil {
			return usagef("--nbf: %v", err)
		}
		t := now.Add(d)
		c.NotBefore = &t
	}
	if err := checkRoles(c.Roles, allowRoles); err != nil {
		return err
	}
	if c.ExpiresAt == nil {
		fmt.Fprintln(os.Stderr, "note: no exp; this ticket never expires (exp is the only bound on reuse)")
	}
	tok, err := mintToken(priv, c)
	if err != nil {
		return err
	}
	if *out != "" {
		return os.WriteFile(*out, []byte(tok+"\n"), 0o600)
	}
	fmt.Println(tok)
	return nil
}

// checkRoles holds roles to the base-profile vocabulary (the server
// refuses unknown roles at admission, 0x1A5A01); --allow-role opens a
// name for a deployment running another profile.
func checkRoles(roles []string, allowed []string) error {
	for _, r := range roles {
		if base.ValidRoles([]string{r}) {
			continue
		}
		ok := false
		for _, a := range allowed {
			if a == r {
				ok = true
			}
		}
		if !ok {
			return fmt.Errorf("role %q is not in the base-profile vocabulary (%s); pass --allow-role %s for a custom profile",
				r, strings.Join([]string{base.RoleAdmin, base.RolePresenter, base.RoleModerator, base.RoleProducer, base.RoleAudience}, ", "), r)
		}
	}
	return nil
}

// parseDuration is time.ParseDuration plus a d (day) suffix.
func parseDuration(s string) (time.Duration, error) {
	if strings.HasSuffix(s, "d") {
		n, err := strconv.ParseFloat(strings.TrimSuffix(s, "d"), 64)
		if err != nil {
			return 0, fmt.Errorf("bad duration %q", s)
		}
		return time.Duration(n * 24 * float64(time.Hour)), nil
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("bad duration %q (use 90m, 24h, 7d)", s)
	}
	return d, nil
}

type multiFlag []string

func (m *multiFlag) String() string     { return strings.Join(*m, ",") }
func (m *multiFlag) Set(s string) error { *m = append(*m, s); return nil }

// mintToken signs c and verifies the result with the public half of the
// same key against c.SpaceID: the printed token is one the server's
// verifier accepts for that space, by construction.
func mintToken(priv ed25519.PrivateKey, c connect.MintClaims) (string, error) {
	tok, err := connect.Mint(priv, c)
	if err != nil {
		return "", err
	}
	v := &connect.Verifier{SpaceID: c.SpaceID, Keys: []ed25519.PublicKey{priv.Public().(ed25519.PublicKey)}}
	if _, rej := v.Verify(tok); rej != nil {
		return "", fmt.Errorf("minted ticket fails verification (not written): %v", rej)
	}
	return tok, nil
}
