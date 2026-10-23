package main

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"flag"
	"fmt"
	"os"
	"strings"
)

// signlic — narzędzie wydawcy: podpisuje licencję premium (hwid + features)
// kluczem prywatnym RSA, zgodnie z weryfikacją w internal/license
// (payload "<hwid>|<feature1>,<feature2>...", PKCS#1 v1.5 + SHA-256).
//
// UWAGA: klucz prywatny trzymany POZA katalogiem dystrybucji aplikacji
// (np. social-downloader/keys/). Kto ma klucz — podpisuje sobie premium.

func loadPrivateKey(path string) (*rsa.PrivateKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("brak bloku PEM w %s", path)
	}
	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("nie można sparsować klucza prywatnego: %w", err)
	}
	rsaKey, ok := key.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("klucz prywatny nie jest kluczem RSA")
	}
	return rsaKey, nil
}

func main() {
	keyPath := flag.String("key", "keys/private-key.pem", "ścieżka do klucza prywatnego PEM")
	hwid := flag.String("hwid", "", "Hardware ID klienta (np. win:<guid>:<host>)")
	features := flag.String("features", "hd", "cechy rozdzielone przecinkiem (np. hd,nowatermark)")
	out := flag.String("out", "license.key", "plik wyjściowy license.key")
	flag.Parse()

	if *hwid == "" {
		fmt.Fprintln(os.Stderr, "błąd: wymagany -hwid (Hardware ID klienta z zakładki licencji)")
		flag.Usage()
		os.Exit(1)
	}
	feats := []string{}
	for _, f := range strings.Split(*features, ",") {
		f = strings.TrimSpace(f)
		if f != "" {
			feats = append(feats, f)
		}
	}

	key, err := loadPrivateKey(*keyPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "błąd: %v\n", err)
		os.Exit(1)
	}

	payload := *hwid + "|" + strings.Join(feats, ",")
	hash := sha256.Sum256([]byte(payload))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, hash[:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "błąd podpisu: %v\n", err)
		os.Exit(1)
	}

	doc := struct {
		HardwareID string   `json:"hwid"`
		Features   []string `json:"features"`
		Signature  string   `json:"signature"`
	}{
		HardwareID: *hwid,
		Features:   feats,
		Signature:  base64.StdEncoding.EncodeToString(sig),
	}
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "błąd serializacji: %v\n", err)
		os.Exit(1)
	}
	if err := os.WriteFile(*out, data, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "błąd zapisu: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("OK: licencja podpisana → %s (hwid=%s, features=%v)\n", *out, *hwid, feats)
}
