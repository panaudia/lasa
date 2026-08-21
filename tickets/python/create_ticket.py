"""Create and sign a LASA ticket with Python.

Format: schemata/ticket.schema.json (lasa-core.md §4.3).
"""

import time
import jwt

# Load your private key
with open("private.pem", "rb") as f:
    private_key = f.read()

headers = {
    "typ": "JWT",
    "alg": "EdDSA",
    "crv": "Ed25519",
}

now = int(time.time())

payload = {
    "iat": now,
    # exp is the only bound on ticket reuse — set it deliberately.
    "exp": now + 86400,
    "iss": "my-app",
    # aud MUST equal the space id: [a-z0-9-] only, no underscores.
    "aud": "df8c7a85-0702-45e9-a626-a1c147eafce9",
    "panaudia": {
        # Identity: the config's client_id must equal this claim.
        "client_id": "6f1c2d3e-8a4b-4c5d-9e0f-1a2b3c4d5e6f",
        "roles": ["presenter"],
        # Explicit state-access grants (key prefixes), union'd with roles.
        "permissions": {
            "set": ["myapp."],
            "clear": ["myapp."],
            "read": ["myapp."],
        },
        # Signed entity definitions, instantiated by the config's setups.
        "entities": [
            {
                "id": "a1b2c3d4-0000-0000-0000-000000000001",
                "name": "Paul",
                "priority": True,
                "heard-in": ["main"],
                "hears": ["main"],
                "attrs": {"colour": "00aaff"},
                "render": {"gain": 1.5, "attenuation": 2.0},
            }
        ],
    },
}

ticket = jwt.encode(payload, private_key, algorithm="EdDSA", headers=headers)
print(ticket)
