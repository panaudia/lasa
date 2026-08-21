// Generate Ed25519 key pair for signing LASA tickets.
// Run with: go run .
package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
)

func main() {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		panic(err)
	}

	// Save private key
	privBytes, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		panic(err)
	}
	privFile, err := os.Create("private.pem")
	if err != nil {
		panic(err)
	}
	pem.Encode(privFile, &pem.Block{Type: "PRIVATE KEY", Bytes: privBytes})
	privFile.Close()

	// Save public key
	pubBytes, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		panic(err)
	}
	pubFile, err := os.Create("public.pem")
	if err != nil {
		panic(err)
	}
	pem.Encode(pubFile, &pem.Block{Type: "PUBLIC KEY", Bytes: pubBytes})
	pubFile.Close()

	fmt.Println("Keys written to private.pem and public.pem")
}
