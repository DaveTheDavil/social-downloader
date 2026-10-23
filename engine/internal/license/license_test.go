package license

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// generateTestKey para RSA + PEM klucza publicznego do testów.
func generateTestKey(t *testing.T) (privateKey *rsa.PrivateKey, publicKeyPEM string) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa.GenerateKey: %v", err)
	}
	der, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatalf("x509.MarshalPKIXPublicKey: %v", err)
	}
	block := &pem.Block{Type: "PUBLIC KEY", Bytes: der}
	publicKeyPEM = string(pem.EncodeToMemory(block))
	return key, publicKeyPEM
}

// signPayload podpisuje ładunek kluczem prywatnym (PKCS#1 v1.5 + SHA-256).
func signPayload(t *testing.T, key *rsa.PrivateKey, payload string) string {
	t.Helper()
	hash := sha256.Sum256([]byte(payload))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, hash[:])
	if err != nil {
		t.Fatalf("rsa.SignPKCS1v15: %v", err)
	}
	return base64.StdEncoding.EncodeToString(sig)
}

func makeLicenseFile(t *testing.T, hwid string, features []string, signature string) string {
	t.Helper()
	// prosty JSON ręcznie, aby zachować kolejność pól jak w specyfikacji
	content := `{"hwid":"` + hwid + `","features":` + featuresJSON(features) + `,"signature":"` + signature + `"}`
	dir := t.TempDir()
	path := filepath.Join(dir, "license.key")
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("zapis pliku licencji: %v", err)
	}
	return path
}

func featuresJSON(features []string) string {
	var b strings.Builder
	b.WriteString("[")
	for i, f := range features {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString(`"` + f + `"`)
	}
	b.WriteString("]")
	return b.String()
}

func TestMachineIDNiePusty(t *testing.T) {
	id, err := MachineID()
	if err != nil {
		t.Fatalf("MachineID zwrócił błąd: %v", err)
	}
	if id == "" {
		t.Fatal("MachineID zwróciło pusty identyfikator")
	}
	t.Logf("MachineID: %s", id)
}

func TestLoadPoprawnyHWIDIPodpis(t *testing.T) {
	key, pubPEM := generateTestKey(t)
	hwid := "win:test-device-123"
	features := []string{"hd", "nowatermark"}
	payload := hwid + "|" + strings.Join(features, ",")
	signature := signPayload(t, key, payload)

	path := makeLicenseFile(t, hwid, features, signature)

	lic, err := Load(path, pubPEM, hwid)
	if err != nil {
		t.Fatalf("Load poprawnej licencji nie powiodło się: %v", err)
	}
	if lic.HardwareID != hwid {
		t.Errorf("HardwareID = %s, chciano %s", lic.HardwareID, hwid)
	}
	if !lic.HasFeature("hd") {
		t.Error("licencja powinna mieć feature 'hd'")
	}
	if !lic.HasFeature("nowatermark") {
		t.Error("licencja powinna mieć feature 'nowatermark'")
	}
	if lic.HasFeature("4k") {
		t.Error("licencja nie powinna mieć feature '4k'")
	}
}

func TestLoadNieprawidłowyHWID(t *testing.T) {
	key, pubPEM := generateTestKey(t)
	features := []string{"hd"}
	payload := "hwid-a|hd"
	signature := signPayload(t, key, payload)

	path := makeLicenseFile(t, "hwid-a", features, signature)

	// inny hwid niż w licencji → błąd
	if _, err := Load(path, pubPEM, "hwid-b"); err == nil {
		t.Error("Load powinno odrzucić licencję z niepasującym HWID")
	}
}

func TestLoadNieprawidłowyPodpis(t *testing.T) {
	_, pubPEM := generateTestKey(t)
	hwid := "hwid-x"
	features := []string{"hd"}
	// podpis wygenerowany innym kluczem → powinien być odrzucony
	otherKey, _ := generateTestKey(t)
	signature := signPayload(t, otherKey, hwid+"|hd")

	path := makeLicenseFile(t, hwid, features, signature)

	if _, err := Load(path, pubPEM, hwid); err == nil {
		t.Error("Load powinno odrzucić licencję z nieprawidłowym podpisem")
	}
}

func TestLoadZamienionaWersja(t *testing.T) {
	key, pubPEM := generateTestKey(t)
	hwid := "hwid-y"
	signature := signPayload(t, key, hwid+"|hd")

	// zmiana features po podpisaniu (tampering) → podpis nie powinien się zgadzać
	path := makeLicenseFile(t, hwid, []string{"hd", "4k"}, signature)

	if _, err := Load(path, pubPEM, hwid); err == nil {
		t.Error("Load powinno odrzucić licencję po tamperingu features")
	}
}

func TestVerifySignatureUszkodzonyBase64(t *testing.T) {
	_, pubPEM := generateTestKey(t)
	lic := License{HardwareID: "h", Features: []string{"f"}, Signature: "!!!not-base64!!!"}
	if err := verifySignature(lic, pubPEM); err == nil {
		t.Error("verifySignature powinno odrzucić uszkodzony base64")
	}
}
