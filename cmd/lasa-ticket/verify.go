package main

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/panaudia/lasa/connect"
)

func cmdVerify(args []string) error {
	fs := flag.NewFlagSet("verify", flag.ContinueOnError)
	pubPath := fs.String("pub", "", "issuer public key (PEM; a private key PEM also works)")
	raw := fs.String("raw", "", "issuer public key as PANAUDIA_TICKET_KEY (base64 raw)")
	space := fs.String("space", "", "space id to bind aud to (default: the token's own aud)")
	ignoreTime := fs.Bool("ignore-time", false, "skip exp/nbf checks (report them instead)")
	pos, err := parseArgs(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 1 {
		return usagef("usage: lasa-ticket verify TOKEN (--pub public.pem | --raw B64) [--space ID] [--ignore-time]")
	}
	var pub ed25519.PublicKey
	switch {
	case *pubPath != "" && *raw != "":
		return usagef("give one of --pub or --raw")
	case *pubPath != "":
		data, err := readArg(*pubPath)
		if err != nil {
			return err
		}
		if pub, err = parsePublicPEM(data); err != nil {
			return fmt.Errorf("--pub: %v", err)
		}
	case *raw != "":
		var err error
		if pub, err = parseRawPublic(*raw); err != nil {
			return err
		}
	default:
		return usagef("--pub or --raw is required")
	}
	tok, err := readToken(pos[0])
	if err != nil {
		return err
	}
	_, payload, err := decodeUnverified(tok)
	if err != nil {
		return err
	}
	spaceID := *space
	if spaceID == "" {
		aud, _ := payload["aud"].(string)
		if aud == "" {
			return fmt.Errorf("token has no string aud; pass --space")
		}
		spaceID = aud
		fmt.Fprintf(os.Stderr, "note: --space not given; binding to the token's own aud %q (checks everything except which space it is for)\n", aud)
	}
	v := &connect.Verifier{SpaceID: spaceID, Keys: []ed25519.PublicKey{pub}}
	if *ignoreTime {
		// Evaluate at a moment inside the token's own validity window so
		// only exp/nbf are neutralised; everything else is checked as is.
		at := time.Now()
		if nbf, ok := numClaim(payload, "nbf"); ok && at.Before(nbf) {
			at = nbf
		}
		if exp, ok := numClaim(payload, "exp"); ok && !at.Before(exp) {
			at = exp.Add(-time.Second)
		}
		v.Now = func() time.Time { return at }
		fmt.Fprintf(os.Stderr, "note: --ignore-time; evaluating as of %s\n", at.UTC().Format(time.RFC3339))
	}
	t, rej := v.Verify(tok)
	if rej != nil {
		return fmt.Errorf("REJECTED (0x%X): %s", rej.Code, rej.Reason)
	}
	fmt.Printf("OK: verified for space %s\n", spaceID)
	printTicket(t)
	printTimes(payload)
	return nil
}

func cmdInspect(args []string) error {
	fs := flag.NewFlagSet("inspect", flag.ContinueOnError)
	asJSON := fs.Bool("json", false, "print header and payload as JSON only")
	pos, err := parseArgs(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 1 {
		return usagef("usage: lasa-ticket inspect TOKEN [--json]")
	}
	tok, err := readToken(pos[0])
	if err != nil {
		return err
	}
	header, payload, err := decodeUnverified(tok)
	if err != nil {
		return err
	}
	if *asJSON {
		out, _ := json.MarshalIndent(map[string]any{"header": header, "payload": payload}, "", "  ")
		fmt.Println(string(out))
		return nil
	}
	fmt.Println("header:")
	printJSON(header, "  ")
	fmt.Println("payload:")
	printJSON(payload, "  ")
	printTimes(payload)
	parts := strings.Split(tok, ".")
	raw, _ := base64.RawURLEncoding.DecodeString(parts[1])
	if err := connect.ValidateClaims(raw); err != nil {
		fmt.Printf("schema: INVALID: %v\n", err)
	} else {
		fmt.Println("schema: valid (ticket.schema.json)")
	}
	fmt.Println("signature: not checked (use verify with the issuer's public key)")
	return nil
}

func decodeUnverified(tok string) (header, payload map[string]any, err error) {
	parts := strings.Split(tok, ".")
	if len(parts) != 3 {
		return nil, nil, fmt.Errorf("not a JWT (want three dot-separated parts, got %d)", len(parts))
	}
	for i, dst := range []*map[string]any{&header, &payload} {
		b, err := base64.RawURLEncoding.DecodeString(parts[i])
		if err != nil {
			return nil, nil, fmt.Errorf("part %d: %v", i+1, err)
		}
		if err := json.Unmarshal(b, dst); err != nil {
			return nil, nil, fmt.Errorf("part %d: %v", i+1, err)
		}
	}
	return header, payload, nil
}

func numClaim(payload map[string]any, key string) (time.Time, bool) {
	f, ok := payload[key].(float64)
	if !ok {
		return time.Time{}, false
	}
	return time.Unix(int64(f), 0), true
}

func printTimes(payload map[string]any) {
	now := time.Now()
	for _, k := range []string{"iat", "nbf", "exp"} {
		t, ok := numClaim(payload, k)
		if !ok {
			if k == "exp" {
				fmt.Println("exp: none (never expires)")
			}
			continue
		}
		rel := humanRel(t.Sub(now))
		fmt.Printf("%s: %s (%s)\n", k, t.UTC().Format(time.RFC3339), rel)
	}
}

func humanRel(d time.Duration) string {
	past := d < 0
	if past {
		d = -d
	}
	d = d.Round(time.Minute)
	var s string
	switch {
	case d >= 48*time.Hour:
		s = fmt.Sprintf("%.1fd", d.Hours()/24)
	case d >= time.Hour:
		s = fmt.Sprintf("%dh%02dm", int(d.Hours()), int(d.Minutes())%60)
	default:
		s = fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if past {
		return s + " ago"
	}
	return "in " + s
}

func printTicket(t *connect.Ticket) {
	fmt.Printf("issuer: %s\nclient_id: %s\nroles: %s\n", t.Issuer, t.ClientID, orNone(t.Roles))
	fmt.Printf("permissions: set=%s clear=%s read=%s\n", orNone(t.Permissions.Set), orNone(t.Permissions.Clear), orNone(t.Permissions.Read))
	if len(t.Entities) == 0 {
		fmt.Println("entities: none")
	}
	for _, e := range t.Entities {
		fmt.Printf("entity: %s %q heard-in=%s hears=%s", e.ID, e.Name, orNone(e.HeardIn), orNone(e.Hears))
		if e.Priority {
			fmt.Print(" priority")
		}
		fmt.Println()
	}
}

func orNone(s []string) string {
	if len(s) == 0 {
		return "-"
	}
	return strings.Join(s, ",")
}

func printJSON(v any, indent string) {
	b, _ := json.MarshalIndent(v, indent, "  ")
	fmt.Println(indent + string(b))
}
