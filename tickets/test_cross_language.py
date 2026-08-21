"""
Cross-language verification test for LASA tickets.

Generates a key pair with Python, then creates tickets with Go and TypeScript,
and verifies that Python can validate the signatures from both — and that each
payload validates against the normative ticket schema
(../schemata/ticket.schema.json), the same check the LASA server applies.
"""

import json
import os
import subprocess
import sys
import tempfile

from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PrivateKey
from cryptography.hazmat.primitives.serialization import (
    Encoding, NoEncryption, PrivateFormat, PublicFormat
)
import jwt
import jsonschema
from referencing import Registry, Resource

SCRIPT_DIR = os.path.dirname(os.path.abspath(__file__))
SCHEMATA_DIR = os.path.join(SCRIPT_DIR, "..", "schemata")
SPACE_ID = "df8c7a85-0702-45e9-a626-a1c147eafce9"
CLIENT_ID = "6f1c2d3e-8a4b-4c5d-9e0f-1a2b3c4d5e6f"
ENTITY_ID = "a1b2c3d4-0000-0000-0000-000000000001"


def load_ticket_validator() -> jsonschema.Draft202012Validator:
    """Load ticket.schema.json with its entity/pose refs resolvable."""
    resources = []
    ticket_schema = None
    for name in ("ticket.schema.json", "entity.schema.json", "pose.schema.json"):
        with open(os.path.join(SCHEMATA_DIR, name)) as f:
            schema = json.load(f)
        resources.append((schema["$id"], Resource.from_contents(schema)))
        if name == "ticket.schema.json":
            ticket_schema = schema
    registry = Registry().with_resources(resources)
    return jsonschema.Draft202012Validator(ticket_schema, registry=registry)


def generate_keys(tmpdir: str) -> tuple[bytes, bytes]:
    """Generate Ed25519 key pair and write PEM files to tmpdir."""
    private_key = Ed25519PrivateKey.generate()

    private_pem = private_key.private_bytes(
        encoding=Encoding.PEM,
        format=PrivateFormat.PKCS8,
        encryption_algorithm=NoEncryption()
    )
    public_pem = private_key.public_key().public_bytes(
        encoding=Encoding.PEM,
        format=PublicFormat.SubjectPublicKeyInfo
    )

    with open(os.path.join(tmpdir, "private.pem"), "wb") as f:
        f.write(private_pem)
    with open(os.path.join(tmpdir, "public.pem"), "wb") as f:
        f.write(public_pem)

    return private_pem, public_pem


def verify_ticket(ticket: str, public_pem: bytes, validator, label: str):
    """Verify signature, check all claims, and validate against the schema."""
    decoded = jwt.decode(
        ticket,
        public_pem,
        algorithms=["EdDSA"],
        audience=SPACE_ID,
    )

    assert decoded["iss"] == "my-app", f"{label}: iss = {decoded['iss']}"
    assert decoded["aud"] == SPACE_ID, f"{label}: aud = {decoded['aud']}"
    assert isinstance(decoded["iat"], int), f"{label}: iat not int"
    assert isinstance(decoded["exp"], int), f"{label}: exp not int"
    assert "jti" not in decoded, f"{label}: jti present — the constrained profile has no jti"

    p = decoded["panaudia"]
    assert p["client_id"] == CLIENT_ID, f"{label}: client_id = {p.get('client_id')}"
    assert p["roles"] == ["presenter"], f"{label}: roles = {p.get('roles')}"
    assert p["permissions"]["set"] == ["myapp."], f"{label}: permissions.set = {p['permissions'].get('set')}"
    assert p["permissions"]["clear"] == ["myapp."], f"{label}: permissions.clear = {p['permissions'].get('clear')}"
    assert p["permissions"]["read"] == ["myapp."], f"{label}: permissions.read = {p['permissions'].get('read')}"
    e = p["entities"][0]
    assert e["id"] == ENTITY_ID, f"{label}: entity.id = {e.get('id')}"
    assert e["name"] == "Paul", f"{label}: entity.name = {e.get('name')}"
    assert e["priority"] is True, f"{label}: entity.priority = {e.get('priority')}"
    assert e["heard-in"] == ["main"], f"{label}: entity.heard-in = {e.get('heard-in')}"
    assert e["hears"] == ["main"], f"{label}: entity.hears = {e.get('hears')}"
    assert e["attrs"]["colour"] == "00aaff", f"{label}: attrs.colour = {e['attrs'].get('colour')}"
    assert e["render"]["gain"] == 1.5, f"{label}: render.gain = {e['render'].get('gain')}"
    assert e["render"]["attenuation"] == 2.0, f"{label}: render.attenuation = {e['render'].get('attenuation')}"

    validator.validate(decoded)

    print(f"  {label}: signature valid, all claims correct, schema valid")


def create_ticket_with_typescript(tmpdir: str) -> str:
    """Run the TypeScript create_ticket script and capture the JWT."""
    ts_dir = os.path.join(SCRIPT_DIR, "typescript")
    # Copy keys into typescript dir
    for f in ("private.pem", "public.pem"):
        src = os.path.join(tmpdir, f)
        dst = os.path.join(ts_dir, f)
        with open(src, "rb") as s, open(dst, "wb") as d:
            d.write(s.read())

    try:
        result = subprocess.run(
            ["npx", "tsx", "create_ticket.mts"],
            cwd=ts_dir,
            capture_output=True,
            text=True,
            timeout=30,
        )
        if result.returncode != 0:
            print(f"  TypeScript stderr: {result.stderr}", file=sys.stderr)
            raise RuntimeError(f"TypeScript failed: {result.stderr}")
        return result.stdout.strip()
    finally:
        for f in ("private.pem", "public.pem"):
            try:
                os.remove(os.path.join(ts_dir, f))
            except FileNotFoundError:
                pass


def create_ticket_with_go(tmpdir: str) -> str:
    """Run the Go create_ticket program and capture the JWT."""
    go_dir = os.path.join(SCRIPT_DIR, "go", "create_ticket")
    # Copy keys into go dir
    for f in ("private.pem", "public.pem"):
        src = os.path.join(tmpdir, f)
        dst = os.path.join(go_dir, f)
        with open(src, "rb") as s, open(dst, "wb") as d:
            d.write(s.read())

    try:
        result = subprocess.run(
            ["go", "run", "."],
            cwd=go_dir,
            capture_output=True,
            text=True,
            timeout=30,
        )
        if result.returncode != 0:
            print(f"  Go stderr: {result.stderr}", file=sys.stderr)
            raise RuntimeError(f"Go failed: {result.stderr}")
        return result.stdout.strip()
    finally:
        for f in ("private.pem", "public.pem"):
            try:
                os.remove(os.path.join(go_dir, f))
            except FileNotFoundError:
                pass


def main():
    validator = load_ticket_validator()
    with tempfile.TemporaryDirectory() as tmpdir:
        # Step 1: Generate keys with Python
        print("Generating keys with Python...")
        private_pem, public_pem = generate_keys(tmpdir)
        print("  Keys generated OK")

        # Step 2: Create ticket with TypeScript, verify with Python
        print("\nCreating ticket with TypeScript...")
        ts_ticket = create_ticket_with_typescript(tmpdir)
        print(f"  Ticket created ({len(ts_ticket)} chars)")
        verify_ticket(ts_ticket, public_pem, validator, "TypeScript → Python")

        # Step 3: Create ticket with Go, verify with Python
        print("\nCreating ticket with Go...")
        go_ticket = create_ticket_with_go(tmpdir)
        print(f"  Ticket created ({len(go_ticket)} chars)")
        verify_ticket(go_ticket, public_pem, validator, "Go → Python")

        print("\n=== CROSS-LANGUAGE: ALL TESTS PASSED ===")


if __name__ == "__main__":
    main()
