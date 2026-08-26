package main

import (
	"bufio"
	"crypto/rand"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/panaudia/lasa/connect"
	"github.com/panaudia/lasa/ident"
)

// new: a question-and-answer walkthrough that writes a claims file (the
// durable artefact; see plan/ticket-cli.md) and, with --mint, signs it.
// Prompts go to stderr so `--out -` leaves stdout as the file alone.
// Plain bufio, defaults in brackets, one screen of questions.
func cmdNew(args []string) error {
	fs := flag.NewFlagSet("new", flag.ContinueOnError)
	out := fs.String("out", "claims.json", "claims file to write (- for stdout)")
	doMint := fs.Bool("mint", false, "also mint a token from the file (needs --key)")
	keyPath := fs.String("key", "", "issuer private key for --mint")
	var allowRoles multiFlag
	fs.Var(&allowRoles, "allow-role", "accept a role outside the base-profile vocabulary (repeatable)")
	pos, err := parseArgs(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 0 {
		return usagef("usage: lasa-ticket new [--out CLAIMS.json] [--mint --key private.pem]")
	}
	if *doMint && *keyPath == "" {
		return usagef("--mint needs --key")
	}
	if *out != "-" {
		if _, err := os.Stat(*out); err == nil {
			return fmt.Errorf("%s exists; choose --out or move it aside", *out)
		}
	}
	var priv []byte
	if *doMint {
		if priv, err = readArg(*keyPath); err != nil {
			return err
		}
		if _, err := parsePrivatePEM(priv); err != nil {
			return fmt.Errorf("--key: %v", err)
		}
	}

	q := &asker{in: bufio.NewReader(os.Stdin), out: os.Stderr}
	c := connect.MintClaims{}
	fmt.Fprintln(q.out, "New LASA ticket claims file (lasa-core.md §4.3). Enter accepts the [default].")
	c.Issuer = q.ask("Issuer (iss)", "my-app", nonEmpty)
	c.SpaceID = q.ask("Space id (aud)", "", identifier)
	c.ClientID = q.ask("Client id", newUUID(), identifier)
	c.Roles = q.askList("Roles (comma separated; empty = audience)", "", func(r string) error {
		return checkRoles([]string{r}, allowRoles)
	})
	c.Permissions.Set = q.askList("State keys the client may set (prefixes, comma separated)", "", keyPrefix)
	c.Permissions.Clear = q.askList("State keys the client may clear", strings.Join(c.Permissions.Set, ","), keyPrefix)
	c.Permissions.Read = q.askList("State keys the client may read beyond its roles", "", keyPrefix)
	for q.yes("Add an entity", len(c.Entities) == 0) {
		e := connect.Entity{}
		e.ID = q.ask("  Entity id", newUUID(), identifier)
		e.Name = q.ask("  Name", "", nonEmpty)
		e.HeardIn = q.askList("  Heard in channels", "main", identifier)
		e.Hears = q.askList("  Hears channels", "main", identifier)
		e.Priority = q.yes("  Priority (protected from moderation)", false)
		c.Entities = append(c.Entities, e)
	}
	exp := q.ask("Expires (duration from now like 24h or 7d, or never)", "24h", func(s string) error {
		if s == "never" {
			return nil
		}
		d, err := parseDuration(s)
		if err != nil {
			return err
		}
		if d <= 0 {
			return fmt.Errorf("must be positive")
		}
		return nil
	})
	if exp != "never" {
		d, _ := parseDuration(exp)
		t := time.Now().Add(d).Truncate(time.Second)
		c.ExpiresAt = &t
	}
	if q.err != nil {
		return q.err
	}

	data, err := connect.FormatClaims(c)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if *out == "-" {
		os.Stdout.Write(data)
	} else {
		if err := os.WriteFile(*out, data, 0o644); err != nil {
			return err
		}
		fmt.Fprintf(q.out, "wrote %s\n", *out)
	}
	if !*doMint {
		if *out != "-" {
			fmt.Fprintf(q.out, "mint with: lasa-ticket mint --key private.pem %s\n", *out)
		}
		return nil
	}
	key, _ := parsePrivatePEM(priv)
	c.IssuedAt = time.Now().Truncate(time.Second)
	tok, err := mintToken(key, c)
	if err != nil {
		return err
	}
	if *out == "-" {
		// The file went to stdout; keep the token separable.
		fmt.Fprintln(os.Stderr, "token:")
	}
	fmt.Println(tok)
	return nil
}

// asker reads answers line by line; the first error (EOF, or a value
// still invalid after a re-ask) sticks, and later questions are skipped.
type asker struct {
	in  *bufio.Reader
	out io.Writer
	err error
}

func (a *asker) line(prompt string) (string, bool) {
	if a.err != nil {
		return "", false
	}
	fmt.Fprint(a.out, prompt)
	s, err := a.in.ReadString('\n')
	if err != nil && (err != io.EOF || s == "") {
		a.err = fmt.Errorf("input ended at %q", strings.TrimSpace(prompt))
		return "", false
	}
	return strings.TrimSpace(s), true
}

// ask repeats until valid; a validator returning an error prints it and
// re-asks. The default is shown in brackets and taken on an empty line.
func (a *asker) ask(label, def string, valid func(string) error) string {
	for {
		p := label
		if def != "" {
			p += " [" + def + "]"
		}
		s, ok := a.line(p + ": ")
		if !ok {
			return ""
		}
		if s == "" {
			s = def
		}
		if err := valid(s); err != nil {
			fmt.Fprintf(a.out, "  %v\n", err)
			continue
		}
		return s
	}
}

func (a *asker) askList(label, def string, validItem func(string) error) []string {
	s := a.ask(label, def, func(s string) error {
		for _, item := range splitList(s) {
			if err := validItem(item); err != nil {
				return fmt.Errorf("%q: %v", item, err)
			}
		}
		return nil
	})
	return splitList(s)
}

func (a *asker) yes(label string, def bool) bool {
	d := "y/N"
	if def {
		d = "Y/n"
	}
	s := a.ask(label+" ("+d+")", map[bool]string{true: "y", false: "n"}[def], func(s string) error {
		switch strings.ToLower(s) {
		case "y", "yes", "n", "no":
			return nil
		}
		return fmt.Errorf("answer y or n")
	})
	return strings.HasPrefix(strings.ToLower(s), "y")
}

func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func nonEmpty(s string) error {
	if s == "" {
		return fmt.Errorf("required")
	}
	return nil
}

func identifier(s string) error { return ident.CheckIdentifier(s) }

func keyPrefix(s string) error {
	if !ident.ValidKeyPrefix(s) {
		return fmt.Errorf("not a key prefix (dot-separated identifiers, e.g. myapp. or myapp.room)")
	}
	return nil
}

// newUUID is a random (version 4) UUID; ids only need the §2 grammar,
// UUIDs are the recommended way to get uniqueness without coordination.
func newUUID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
