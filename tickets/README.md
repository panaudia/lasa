# Tickets

A LASA ticket is a signed JWT (EdDSA / Ed25519) that admits one client to
one space, carrying its identity, roles, state permissions and signed
entity definitions. Format: `../lasa-core.md` §4.3, schema
`../schemata/ticket.schema.json`. The claim set is closed: `iss`, `aud`
(the space id, a plain string), optional `exp` / `nbf` / `iat`, and
`panaudia { client_id, roles, permissions, entities }`. Nothing else.

Two things live here:

- **`lasa-ticket`**, the command-line tool (source in `../cmd/lasa-ticket`),
  for operators and developers: keys, minting, checking. It is built on
  `connect`, the package the server verifies with, so a ticket it
  verifies is one the server admits.
- **Samples** in `go/`, `python/`, `typescript/`: minimal programs that
  generate a key pair and mint one ticket, for anyone wiring ticket
  issuing into their own backend. `test_cross_language.py` mints with
  each, checks every result in Python and with `lasa-ticket verify`.

## The tool

```
go install github.com/panaudia/lasa/cmd/lasa-ticket@latest
```

**1. Keys.** One Ed25519 pair per space. The server takes the public key
as `PANAUDIA_TICKET_KEY` (base64 of the raw 32 bytes); `keygen` prints
that line so it is never converted by hand.

```
$ lasa-ticket keygen --out keys
wrote keys/private.pem (0600) and keys/public.pem
PANAUDIA_TICKET_KEY=noAIRGAMpAhKIkJ3ajp4HeRAvw7hnHv8jwAyVVahPIc=
```

`lasa-ticket pubkey keys/private.pem` prints the same line for an
existing key.

**2. A claims file.** The file is the token's payload minus `iat`, which
is stamped at minting. Either answer the walkthrough:

```
$ lasa-ticket new --out claims.json            # add --mint --key keys/private.pem to sign it too
Issuer (iss) [my-app]:
Space id (aud): df8c7a85-0702-45e9-a626-a1c147eafce9
Client id [6f1c2d3e-…]:
Roles (comma separated; empty = audience): presenter
…
```

or start from the annotated example and delete the `_comment` keys
(unknown keys are refused, so the file you mint is exactly the payload
you get):

```
$ lasa-ticket example > claims.json
```

The walkthrough validates every answer as the server would (identifier
grammar, role vocabulary, key prefixes) and re-asks; Enter takes the
`[default]`; a fresh UUID is offered for each id.

`aud` must equal the space id. `exp` is epoch seconds and the only bound
on reuse; leave it out of the file and give `--exp` at minting if the
lifetime is a minting-time decision. Roles come from the base profile:
`admin`, `presenter`, `moderator`, `producer`, `audience` (empty means
audience). Entities are the signed definitions the client instantiates by
id in its connection config.

**3. Mint.**

```
$ lasa-ticket mint --key keys/private.pem claims.json --exp 24h > ticket.jwt
```

The claims are decoded, `iat` stamped, the token signed, then verified
with the public half of the same key against the file's `aud`. A token
that would be rejected is never printed; the message is the server's own
rejection reason. Durations are `90m`, `24h`, `7d`; `--nbf` and `--iss`
work the same way.

**4. Check.**

```
$ lasa-ticket inspect ticket.jwt          # decode, times, schema verdict; no key needed
$ lasa-ticket verify ticket.jwt --pub keys/public.pem --space <space id>
$ lasa-ticket verify ticket.jwt --raw "$PANAUDIA_TICKET_KEY"
```

`verify` runs the server's verifier and prints the parsed ticket, or
`REJECTED (0x1A5A01): …` with the reason. Without `--space` it binds to
the token's own `aud` and says so. `--ignore-time` evaluates inside the
token's own validity window, for looking at expired tickets.

`TOKEN` and `CLAIMS.json` may be `-` for stdin, so
`lasa-ticket mint … | lasa-ticket inspect -` works. Exit status is 0 for
ok, 1 for a rejected ticket or bad input, 2 for a usage error.

## The samples

Each of `go/`, `python/`, `typescript/` has `generate_keys` and
`create_ticket`, fixed-value programs with the same claims as
`lasa-ticket example`. They read `private.pem` from their own directory
and print the token. They are reference code, not tools; copy the ten
relevant lines into your issuer.

`test_cross_language.py` (Python 3, `cryptography`, `PyJWT`,
`jsonschema`, `referencing`; Go and Node on the path) generates keys in
Python, mints with TypeScript, Go and `lasa-ticket`, verifies all three
in Python against the schema, and runs `lasa-ticket verify` over the
sample tickets. Run it after touching any of them:

```
$ cd tickets && python3 test_cross_language.py
```
