"""Generate Ed25519 key pair for signing LASA tickets."""

from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PrivateKey
from cryptography.hazmat.primitives.serialization import (
    Encoding, NoEncryption, PrivateFormat, PublicFormat
)

private_key = Ed25519PrivateKey.generate()

# Save private key (keep secret)
private_pem = private_key.private_bytes(
    encoding=Encoding.PEM,
    format=PrivateFormat.PKCS8,
    encryption_algorithm=NoEncryption()
)

# Save public key (give to the Space)
public_pem = private_key.public_key().public_bytes(
    encoding=Encoding.PEM,
    format=PublicFormat.SubjectPublicKeyInfo
)

with open("private.pem", "wb") as f:
    f.write(private_pem)
with open("public.pem", "wb") as f:
    f.write(public_pem)

print("Keys written to private.pem and public.pem")
