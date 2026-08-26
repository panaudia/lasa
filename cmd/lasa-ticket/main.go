// Command lasa-ticket creates keys, mints and checks LASA tickets
// (lasa-core.md §4.3). It is built on the connect package the server
// verifies with, so a ticket this tool verifies is one the server admits.
// Decision record: lasa-planning/plan/ticket-cli.md.
//
// Usage:
//
//	lasa-ticket keygen [--out DIR]
//	lasa-ticket pubkey KEY.pem
//	lasa-ticket mint --key private.pem CLAIMS.json [--exp DUR] [--nbf DUR] [--iss NAME] [--out FILE]
//	lasa-ticket verify TOKEN (--pub public.pem | --raw B64) [--space ID] [--ignore-time]
//	lasa-ticket inspect TOKEN
//	lasa-ticket new [--out CLAIMS.json] [--mint --key private.pem]
//	lasa-ticket example
//
// TOKEN and CLAIMS.json may be "-" for stdin. Exit status: 0 ok, 1 the
// ticket or input was rejected, 2 usage.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
)

const usage = `lasa-ticket: keys, minting and checking for LASA tickets

  keygen  [--out DIR]                       new Ed25519 key pair + PANAUDIA_TICKET_KEY
  pubkey  KEY.pem                           PANAUDIA_TICKET_KEY value for an existing key
  mint    --key private.pem CLAIMS.json     sign a claims file (validated, then self-verified)
          [--exp DUR] [--nbf DUR] [--iss NAME] [--out FILE]
  verify  TOKEN (--pub public.pem | --raw B64) [--space ID] [--ignore-time]
                                            the server's verdict on a ticket
  inspect TOKEN                             decode without a key
  new     [--out CLAIMS.json] [--mint --key private.pem]
                                            walkthrough that writes a claims file
  example                                   an annotated claims file to start from

TOKEN and CLAIMS.json may be "-" for stdin. DUR is like 90m, 24h or 7d.
Exit status: 0 ok, 1 rejected or invalid input, 2 usage.`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, usage)
		os.Exit(2)
	}
	cmd, args := os.Args[1], os.Args[2:]
	var err error
	switch cmd {
	case "keygen":
		err = cmdKeygen(args)
	case "pubkey":
		err = cmdPubkey(args)
	case "mint":
		err = cmdMint(args)
	case "verify":
		err = cmdVerify(args)
	case "inspect":
		err = cmdInspect(args)
	case "new":
		err = cmdNew(args)
	case "example":
		err = cmdExample(args)
	case "help", "-h", "--help":
		fmt.Println(usage)
		return
	default:
		fmt.Fprintf(os.Stderr, "lasa-ticket: unknown command %q\n\n%s\n", cmd, usage)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "lasa-ticket %s: %v\n", cmd, err)
		var u usageError
		if isUsage(err, &u) {
			os.Exit(2)
		}
		os.Exit(1)
	}
}

// usageError marks bad invocation (exit 2) as opposed to a rejected
// ticket or unreadable input (exit 1).
type usageError struct{ msg string }

func (u usageError) Error() string { return u.msg }

func usagef(format string, a ...any) error { return usageError{fmt.Sprintf(format, a...)} }

func isUsage(err error, u *usageError) bool {
	ue, ok := err.(usageError)
	if ok {
		*u = ue
	}
	return ok
}

// readArg reads a positional file argument, "-" meaning stdin. Tokens
// are trimmed; JSON is returned as is.
func readArg(path string) ([]byte, error) {
	if path == "-" {
		return io.ReadAll(os.Stdin)
	}
	return os.ReadFile(path)
}

func readToken(path string) (string, error) {
	b, err := readArg(path)
	if err != nil {
		return "", err
	}
	tok := strings.TrimSpace(string(b))
	if tok == "" {
		return "", fmt.Errorf("empty token")
	}
	return tok, nil
}

// parseArgs parses flags and positionals in any order (the flag package
// stops at the first positional), returning the positionals.
func parseArgs(fs *flag.FlagSet, args []string) ([]string, error) {
	var pos []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, usagef("%v", err)
		}
		rest := fs.Args()
		if len(rest) == 0 {
			return pos, nil
		}
		if rest[0] == "--" {
			return append(pos, rest[1:]...), nil
		}
		pos = append(pos, rest[0])
		args = rest[1:]
	}
}
