package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
)

const keyBits = 2048

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "Użycie: %s <katalog-wyjsciowy>\n", os.Args[0])
		os.Exit(1)
	}
	outDir := os.Args[1]

	if err := os.MkdirAll(outDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "Błąd tworzenia katalogu wyjściowego: %v\n", err)
		os.Exit(1)
	}

	privateKey, err := rsa.GenerateKey(rand.Reader, keyBits)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Błąd generowania klucza prywatnego RSA: %v\n", err)
		os.Exit(1)
	}
	if err := privateKey.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "Wygenerowany klucz prywatny jest niepoprawny: %v\n", err)
		os.Exit(1)
	}

	privDER, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Błąd serializacji klucza prywatnego: %v\n", err)
		os.Exit(1)
	}
	privBlock := &pem.Block{Type: "PRIVATE KEY", Bytes: privDER}
	privFile := filepath.Join(outDir, "private-key.pem")
	if err := os.WriteFile(privFile, pem.EncodeToMemory(privBlock), 0o600); err != nil {
		fmt.Fprintf(os.Stderr, "Błąd zapisu klucza prywatnego: %v\n", err)
		os.Exit(1)
	}

	pubDER, err := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Błąd serializacji klucza publicznego: %v\n", err)
		os.Exit(1)
	}
	pubBlock := &pem.Block{Type: "PUBLIC KEY", Bytes: pubDER}
	pubFile := filepath.Join(outDir, "public-key.pem")
	if err := os.WriteFile(pubFile, pem.EncodeToMemory(pubBlock), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "Błąd zapisu klucza publicznego: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Wygenerowano klucze RSA %d bitów w katalogu %s\n", keyBits, outDir)
	fmt.Printf("  %s\n", privFile)
	fmt.Printf("  %s\n", pubFile)
}
