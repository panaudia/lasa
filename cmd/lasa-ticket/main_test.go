package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/panaudia/lasa/connect"
)

// The CLI is exercised as a built binary so exit codes, stdin handling and
// flag order are what a user gets. TestMain builds it once.
var bin string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "lasa-ticket-test")
	if err != nil {
		panic(err)
	}
	bin = filepath.Join(dir, "lasa-ticket")
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		panic(string(out))
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

type result struct {
	stdout, stderr string
	code           int
}

func run(t *testing.T, stdin string, args ...string) result {
	t.Helper()
	cmd := exec.Command(bin, args...)
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	var so, se strings.Builder
	cmd.Stdout, cmd.Stderr = &so, &se
	err := cmd.Run()
	code := 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatal(err)
	}
	return result{so.String(), se.String(), code}
}

const claims = `{
  "iss": "test",
  "aud": "space-1",
  "panaudia": {
    "client_id": "client-1",
    "roles": ["presenter"],
    "entities": [{"id": "ent-1", "name": "One", "heard-in": ["main"]}]
  }
}`

// setup: a key pair via keygen and a claims file; returns the dir.
func setup(t *testing.T) (dir, raw string) {
	t.Helper()
	dir = t.TempDir()
	r := run(t, "", "keygen", "--out", filepath.Join(dir, "k"))
	if r.code != 0 {
		t.Fatalf("keygen: %d %s", r.code, r.stderr)
	}
	raw = strings.TrimSpace(strings.TrimPrefix(r.stdout, "PANAUDIA_TICKET_KEY="))
	if err := os.WriteFile(filepath.Join(dir, "claims.json"), []byte(claims), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir, raw
}

func TestKeygenAndPubkey(t *testing.T) {
	dir, raw := setup(t)
	if _, err := base64.StdEncoding.DecodeString(raw); err != nil || len(raw) != 44 {
		t.Fatalf("raw key %q", raw)
	}
	st, err := os.Stat(filepath.Join(dir, "k", "private.pem"))
	if err != nil || st.Mode().Perm() != 0o600 {
		t.Fatalf("private.pem mode %v err %v", st.Mode(), err)
	}
	if r := run(t, "", "keygen", "--out", filepath.Join(dir, "k")); r.code != 1 || !strings.Contains(r.stderr, "refusing to overwrite") {
		t.Fatalf("second keygen: %d %s", r.code, r.stderr)
	}
	for _, f := range []string{"private.pem", "public.pem"} {
		r := run(t, "", "pubkey", filepath.Join(dir, "k", f))
		if r.code != 0 || strings.TrimSpace(r.stdout) != "PANAUDIA_TICKET_KEY="+raw {
			t.Fatalf("pubkey %s: %d %q", f, r.code, r.stdout)
		}
	}
}

func TestMintVerifyInspect(t *testing.T) {
	dir, raw := setup(t)
	key := filepath.Join(dir, "k", "private.pem")
	pub := filepath.Join(dir, "k", "public.pem")
	cf := filepath.Join(dir, "claims.json")

	// flags after the positional, --out, exp as duration
	r := run(t, "", "mint", "--key", key, cf, "--exp", "2h", "--out", filepath.Join(dir, "t.jwt"))
	if r.code != 0 {
		t.Fatalf("mint: %d %s", r.code, r.stderr)
	}
	tok, _ := os.ReadFile(filepath.Join(dir, "t.jwt"))
	if strings.Count(strings.TrimSpace(string(tok)), ".") != 2 {
		t.Fatalf("token %q", tok)
	}

	// verify from file with PEM, default space from aud
	r = run(t, "", "verify", filepath.Join(dir, "t.jwt"), "--pub", pub)
	if r.code != 0 || !strings.Contains(r.stdout, "OK: verified for space space-1") || !strings.Contains(r.stdout, "client_id: client-1") {
		t.Fatalf("verify: %d %s %s", r.code, r.stdout, r.stderr)
	}
	if !strings.Contains(r.stderr, "--space not given") {
		t.Fatalf("expected the aud note, got %q", r.stderr)
	}
	// verify from stdin with raw key (padded and unpadded), explicit space
	for _, k := range []string{raw, strings.TrimRight(raw, "=")} {
		r = run(t, string(tok), "verify", "-", "--raw", k, "--space", "space-1")
		if r.code != 0 || !strings.HasPrefix(r.stdout, "OK") {
			t.Fatalf("verify --raw %q: %d %s", k, r.code, r.stderr)
		}
	}
	// wrong space → rejection with the ticket code
	r = run(t, "", "verify", filepath.Join(dir, "t.jwt"), "--pub", pub, "--space", "other")
	if r.code != 1 || !strings.Contains(r.stderr, "REJECTED (0x1A5A01)") || !strings.Contains(r.stderr, "audience") {
		t.Fatalf("wrong space: %d %s", r.code, r.stderr)
	}
	// wrong key
	other := t.TempDir()
	run(t, "", "keygen", "--out", other)
	r = run(t, "", "verify", filepath.Join(dir, "t.jwt"), "--pub", filepath.Join(other, "public.pem"))
	if r.code != 1 || !strings.Contains(r.stderr, "signature is invalid") {
		t.Fatalf("wrong key: %d %s", r.code, r.stderr)
	}
	// inspect: no key, schema verdict, times
	r = run(t, string(tok), "inspect", "-")
	if r.code != 0 || !strings.Contains(r.stdout, "schema: valid") || !strings.Contains(r.stdout, "exp: ") || !strings.Contains(r.stdout, "in 2h00m") {
		t.Fatalf("inspect: %d %s", r.code, r.stdout)
	}
	r = run(t, string(tok), "inspect", "-", "--json")
	if r.code != 0 || !strings.Contains(r.stdout, `"alg": "EdDSA"`) || !strings.Contains(r.stdout, `"aud": "space-1"`) {
		t.Fatalf("inspect --json: %d %s", r.code, r.stdout)
	}
}

func TestMintRejectsBadInput(t *testing.T) {
	dir, _ := setup(t)
	key := filepath.Join(dir, "k", "private.pem")
	cf := filepath.Join(dir, "claims.json")

	if r := run(t, "", "mint", "--key", key, cf, "--exp", "-1h"); r.code != 2 || !strings.Contains(r.stderr, "positive") {
		t.Fatalf("negative exp: %d %s", r.code, r.stderr)
	}
	if r := run(t, "", "mint", "--key", key, cf, "--exp", "soon"); r.code != 2 {
		t.Fatalf("bad duration: %d %s", r.code, r.stderr)
	}
	if r := run(t, "", "mint", cf); r.code != 2 || !strings.Contains(r.stderr, "--key is required") {
		t.Fatalf("no key: %d %s", r.code, r.stderr)
	}
	bad := strings.Replace(claims, `"presenter"`, `"dj"`, 1)
	r := run(t, bad, "mint", "--key", key, "-")
	if r.code != 1 || !strings.Contains(r.stderr, `role "dj"`) {
		t.Fatalf("bad role: %d %s", r.code, r.stderr)
	}
	if r := run(t, bad, "mint", "--key", key, "-", "--allow-role", "dj"); r.code != 0 {
		t.Fatalf("allow-role: %d %s", r.code, r.stderr)
	}
	// claims that pass ParseClaims but fail the schema (client_id grammar)
	bad = strings.Replace(claims, `"client-1"`, `"Client_1"`, 1)
	if r := run(t, bad, "mint", "--key", key, "-"); r.code != 1 || !strings.Contains(r.stderr, "fails verification") {
		t.Fatalf("schema-invalid claims: %d %s", r.code, r.stderr)
	}
	// iat in the file
	bad = strings.Replace(claims, `"iss": "test",`, `"iss": "test", "iat": 1,`, 1)
	if r := run(t, bad, "mint", "--key", key, "-"); r.code != 1 || !strings.Contains(r.stderr, "iat") {
		t.Fatalf("iat in file: %d %s", r.code, r.stderr)
	}
	// no exp: minted, with a note
	if r := run(t, "", "mint", "--key", key, cf); r.code != 0 || !strings.Contains(r.stderr, "never expires") {
		t.Fatalf("no exp: %d %s", r.code, r.stderr)
	}
}

// An expired ticket cannot be minted by the CLI (self-verification refuses
// it), so build one with connect.Mint to exercise --ignore-time.
func TestVerifyIgnoreTime(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	dir := t.TempDir()
	der, _ := x509.MarshalPKIXPublicKey(pub)
	pubPath := filepath.Join(dir, "public.pem")
	os.WriteFile(pubPath, pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}), 0o644)
	c, err := connect.ParseClaims([]byte(claims))
	if err != nil {
		t.Fatal(err)
	}
	c.IssuedAt = time.Now().Add(-2 * time.Hour)
	exp := time.Now().Add(-time.Hour)
	c.ExpiresAt = &exp
	tok, err := connect.Mint(priv, c)
	if err != nil {
		t.Fatal(err)
	}
	if r := run(t, tok, "verify", "-", "--pub", pubPath); r.code != 1 || !strings.Contains(r.stderr, "expired") {
		t.Fatalf("expired: %d %s", r.code, r.stderr)
	}
	r := run(t, tok, "verify", "-", "--pub", pubPath, "--ignore-time")
	if r.code != 0 || !strings.HasPrefix(r.stdout, "OK") || !strings.Contains(r.stdout, "ago)") {
		t.Fatalf("ignore-time: %d %s %s", r.code, r.stdout, r.stderr)
	}
	if !strings.Contains(r.stderr, "evaluating as of") {
		t.Fatalf("expected the ignore-time note, got %q", r.stderr)
	}
}

func TestUsage(t *testing.T) {
	if r := run(t, ""); r.code != 2 {
		t.Fatalf("no args: %d", r.code)
	}
	if r := run(t, "", "bogus"); r.code != 2 || !strings.Contains(r.stderr, "unknown command") {
		t.Fatalf("unknown: %d %s", r.code, r.stderr)
	}
	if r := run(t, "", "help"); r.code != 0 || !strings.Contains(r.stdout, "keygen") {
		t.Fatalf("help: %d", r.code)
	}
	if r := run(t, "", "example"); r.code != 0 || !strings.Contains(r.stdout, `"panaudia"`) {
		t.Fatalf("example: %d", r.code)
	}
	if r := run(t, "abc", "inspect", "-"); r.code != 1 || !strings.Contains(r.stderr, "not a JWT") {
		t.Fatalf("garbage: %d %s", r.code, r.stderr)
	}
}

// The walkthrough is driven by scripted stdin. Answers, in order: iss,
// aud, client id, roles, set, clear, read, add entity?, entity id, name,
// heard-in, hears, priority?, add another?, expiry.
const walkthrough = "\nspace-1\nclient-1\npresenter\nmyapp.\n\n\ny\nent-1\nPaul\nmain,stage\n\ny\nn\n7d\n"

func TestNewWalkthrough(t *testing.T) {
	dir, _ := setup(t)
	key := filepath.Join(dir, "k", "private.pem")
	pub := filepath.Join(dir, "k", "public.pem")
	out := filepath.Join(dir, "new.json")

	r := run(t, walkthrough, "new", "--out", out, "--mint", "--key", key)
	if r.code != 0 {
		t.Fatalf("new: %d %s", r.code, r.stderr)
	}
	// stdout is the token alone; the file is the claims
	if strings.Count(strings.TrimSpace(r.stdout), ".") != 2 || strings.Contains(r.stdout, "{") {
		t.Fatalf("stdout should be just the token: %q", r.stdout)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	c, err := connect.ParseClaims(data)
	if err != nil {
		t.Fatalf("file does not parse: %v\n%s", err, data)
	}
	if c.Issuer != "my-app" || c.SpaceID != "space-1" || c.ClientID != "client-1" || c.Roles[0] != "presenter" {
		t.Fatalf("claims %+v", c)
	}
	if c.Permissions.Set[0] != "myapp." || c.Permissions.Clear[0] != "myapp." || len(c.Permissions.Read) != 0 {
		t.Fatalf("permissions %+v (clear should default to set)", c.Permissions)
	}
	e := c.Entities[0]
	if len(c.Entities) != 1 || e.ID != "ent-1" || e.Name != "Paul" || !e.Priority || len(e.HeardIn) != 2 || e.Hears[0] != "main" {
		t.Fatalf("entities %+v", c.Entities)
	}
	if c.ExpiresAt == nil || time.Until(*c.ExpiresAt) < 6*24*time.Hour {
		t.Fatalf("exp %v", c.ExpiresAt)
	}
	// key order is schema order, not alphabetical
	if !strings.HasPrefix(string(data), "{\n  \"iss\"") {
		t.Fatalf("file should start with iss:\n%s", data)
	}
	// the minted token verifies, and the file re-mints to an equivalent one
	if r := run(t, r.stdout, "verify", "-", "--pub", pub); r.code != 0 {
		t.Fatalf("verify minted: %d %s", r.code, r.stderr)
	}
	if r := run(t, "", "mint", "--key", key, out); r.code != 0 {
		t.Fatalf("re-mint from file: %d %s", r.code, r.stderr)
	}
	// refuses to overwrite
	if r := run(t, walkthrough, "new", "--out", out); r.code != 1 || !strings.Contains(r.stderr, "exists") {
		t.Fatalf("overwrite: %d %s", r.code, r.stderr)
	}
}

func TestNewReasksAndAborts(t *testing.T) {
	// invalid space id, then a bad role, then a bad duration: each re-asked
	script := "\nBad Space\nspace-1\n\ndj\n\n\n\n\nn\nsoon\nnever\n"
	r := run(t, script, "new", "--out", "-")
	if r.code != 0 {
		t.Fatalf("new: %d %s", r.code, r.stderr)
	}
	for _, want := range []string{`invalid identifier "Bad Space"`, `role "dj"`, `bad duration "soon"`} {
		if !strings.Contains(r.stderr, want) {
			t.Errorf("expected re-ask message %q in %q", want, r.stderr)
		}
	}
	c, err := connect.ParseClaims([]byte(r.stdout))
	if err != nil {
		t.Fatalf("stdout is not a claims file: %v\n%s", err, r.stdout)
	}
	if c.SpaceID != "space-1" || len(c.Roles) != 0 || c.ExpiresAt != nil || len(c.Entities) != 0 {
		t.Fatalf("claims %+v", c)
	}
	// input ending early: exit 1, nothing written
	out := filepath.Join(t.TempDir(), "x.json")
	if r := run(t, "x\n", "new", "--out", out); r.code != 1 || !strings.Contains(r.stderr, "input ended") {
		t.Fatalf("eof: %d %s", r.code, r.stderr)
	}
	if _, err := os.Stat(out); err == nil {
		t.Fatal("file written despite abort")
	}
	if r := run(t, "", "new", "--mint"); r.code != 2 {
		t.Fatalf("--mint without --key: %d", r.code)
	}
}
