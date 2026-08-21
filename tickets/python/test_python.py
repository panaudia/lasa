"""Verify Python LASA ticket creation and signing."""

import time
import json
import base64

from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PrivateKey
from cryptography.hazmat.primitives.serialization import (
    Encoding, NoEncryption, PrivateFormat, PublicFormat
)
import jwt

# --- Step 1: Generate keys ---

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

print("Keys generated OK")

# --- Step 2: Create and sign ticket ---

headers = {
    "typ": "JWT",
    "alg": "EdDSA",
    "crv": "Ed25519",
}

now = int(time.time())
SPACE_ID = "df8c7a85-0702-45e9-a626-a1c147eafce9"
CLIENT_ID = "6f1c2d3e-8a4b-4c5d-9e0f-1a2b3c4d5e6f"
ENTITY_ID = "a1b2c3d4-0000-0000-0000-000000000001"

payload = {
    "iat": now,
    "exp": now + 3600,
    "iss": "test-verify",
    "aud": SPACE_ID,
    "panaudia": {
        "client_id": CLIENT_ID,
        "roles": ["presenter"],
        "permissions": {
            "set": ["myapp."],
            "clear": ["myapp."],
            "read": ["myapp."],
        },
        "entities": [
            {
                "id": ENTITY_ID,
                "name": "TestUser",
                "priority": True,
                "heard-in": ["main"],
                "hears": ["main"],
                "attrs": {"colour": "00aaff"},
                "render": {"gain": 1.5, "attenuation": 2.0},
            }
        ],
    },
}

ticket = jwt.encode(payload, private_pem, algorithm="EdDSA", headers=headers)
print(f"Ticket created OK ({len(ticket)} chars)")

# --- Step 3: Verify ---

# Decode without verification to inspect structure
parts = ticket.split(".")
assert len(parts) == 3, f"Expected 3 parts, got {len(parts)}"

# Decode header
header_json = json.loads(base64.urlsafe_b64decode(parts[0] + "=="))
assert header_json["typ"] == "JWT", f"typ: {header_json.get('typ')}"
assert header_json["alg"] == "EdDSA", f"alg: {header_json.get('alg')}"
assert header_json["crv"] == "Ed25519", f"crv: {header_json.get('crv')}"
print(f"Header OK: {header_json}")

# Decode payload
payload_json = json.loads(base64.urlsafe_b64decode(parts[1] + "=="))
assert payload_json["iss"] == "test-verify"
assert payload_json["aud"] == SPACE_ID, "aud must be a plain string (schema forbids array)"
assert payload_json["iat"] == now
assert payload_json["exp"] == now + 3600
p = payload_json["panaudia"]
assert p["client_id"] == CLIENT_ID
assert p["roles"] == ["presenter"]
assert p["permissions"]["set"] == ["myapp."]
assert p["permissions"]["clear"] == ["myapp."]
assert p["permissions"]["read"] == ["myapp."]
e = p["entities"][0]
assert e["id"] == ENTITY_ID
assert e["name"] == "TestUser"
assert e["priority"] is True
assert e["heard-in"] == ["main"]
assert e["hears"] == ["main"]
assert e["attrs"]["colour"] == "00aaff"
assert e["render"]["gain"] == 1.5
assert e["render"]["attenuation"] == 2.0
print("Payload OK: all claims verified")

# Verify signature with public key
decoded = jwt.decode(ticket, public_pem, algorithms=["EdDSA"], audience=SPACE_ID)
print("Signature verified OK with public key")

print("\n=== PYTHON: ALL TESTS PASSED ===")
