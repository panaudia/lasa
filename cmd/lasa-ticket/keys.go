package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Key files are PKCS#8 (private) and PKIX/SPKI (public) PEM, the format
// the tickets/ samples in all three languages read. The server takes
// the public key as base64 of the raw 32 bytes (PANAUDIA_TICKET_KEY);
// keygen and pubkey print that form so nobody converts it by hand.

func cmdKeygen(args []string) error {
	fs := flag.NewFlagSet("keygen", flag.ContinueOnError)
	out := fs.String("out", ".", "directory to write private.pem and public.pem into")
	pos, err := parseArgs(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 0 {
		return usagef("keygen takes no positional arguments")
	}
	privPath := filepath.Join(*out, "private.pem")
	pubPath := filepath.Join(*out, "public.pem")
	for _, p := range []string{privPath, pubPath} {
		if _, err := os.Stat(p); err == nil {
			return fmt.Errorf("%s exists; refusing to overwrite a key", p)
		}
	}
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return err
	}
	privDER, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return err
	}
	pubDER, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(*out, 0o755); err != nil {
		return err
	}
	if err := writePEM(privPath, "PRIVATE KEY", privDER, 0o600); err != nil {
		return err
	}
	if err := writePEM(pubPath, "PUBLIC KEY", pubDER, 0o644); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "wrote %s (0600) and %s\n", privPath, pubPath)
	fmt.Printf("PANAUDIA_TICKET_KEY=%s\n", rawPublicB64(pub))
	return nil
}

func cmdPubkey(args []string) error {
	if len(args) != 1 {
		return usagef("usage: lasa-ticket pubkey KEY.pem")
	}
	data, err := readArg(args[0])
	if err != nil {
		return err
	}
	pub, err := parsePublicPEM(data)
	if err != nil {
		return err
	}
	fmt.Printf("PANAUDIA_TICKET_KEY=%s\n", rawPublicB64(pub))
	return nil
}

func writePEM(path, typ string, der []byte, mode os.FileMode) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	defer f.Close()
	return pem.Encode(f, &pem.Block{Type: typ, Bytes: der})
}

func rawPublicB64(pub ed25519.PublicKey) string {
	return base64.StdEncoding.EncodeToString(pub)
}

// parsePrivatePEM accepts a PKCS#8 Ed25519 private key.
func parsePrivatePEM(data []byte) (ed25519.PrivateKey, error) {
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, errors.New("not a PEM file")
	}
	if block.Type != "PRIVATE KEY" {
		return nil, fmt.Errorf("want a PKCS#8 \"PRIVATE KEY\" block, got %q", block.Type)
	}
	k, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	priv, ok := k.(ed25519.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("not an Ed25519 key (%T); tickets are EdDSA only", k)
	}
	return priv, nil
}

// parsePublicPEM accepts a PKIX "PUBLIC KEY" block, or a PKCS#8 private
// key from which the public half is derived.
func parsePublicPEM(data []byte) (ed25519.PublicKey, error) {
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, errors.New("not a PEM file")
	}
	switch block.Type {
	case "PUBLIC KEY":
		k, err := x509.ParsePKIXPublicKey(block.Bytes)
		if err != nil {
			return nil, err
		}
		pub, ok := k.(ed25519.PublicKey)
		if !ok {
			return nil, fmt.Errorf("not an Ed25519 key (%T)", k)
		}
		return pub, nil
	case "PRIVATE KEY":
		priv, err := parsePrivatePEM(data)
		if err != nil {
			return nil, err
		}
		return priv.Public().(ed25519.PublicKey), nil
	}
	return nil, fmt.Errorf("unexpected PEM block %q", block.Type)
}

// parseRawPublic accepts the PANAUDIA_TICKET_KEY form.
func parseRawPublic(b64 string) (ed25519.PublicKey, error) {
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		// Tolerate a copy that lost its trailing padding.
		raw, err = base64.RawStdEncoding.DecodeString(strings.TrimRight(b64, "="))
		if err != nil {
			return nil, fmt.Errorf("--raw: %v", err)
		}
	}
	if len(raw) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("--raw: want %d key bytes, got %d", ed25519.PublicKeySize, len(raw))
	}
	return ed25519.PublicKey(raw), nil
}
