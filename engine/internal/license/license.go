package license

import (
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// License reprezentuje klucz premium powiązany z konkretnym urządzeniem.
type License struct {
	HardwareID string   `json:"hwid"`
	Features   []string `json:"features"` // np. ["hd", "nowatermark"]
	Signature  string   `json:"signature"`
}

// MachineID generuje deterministyczny identyfikator sprzętu.
// Windows: MachineGuid z rejestru. Inne platformy: kombinacja OS + architektura + hostname.
func MachineID() (string, error) {
	if runtime.GOOS == "windows" {
		id, err := windowsMachineGuid()
		if err == nil && id != "" {
			hostname, _ := os.Hostname()
			return "win:" + id + ":" + hostname, nil
		}
	}
	hostname, _ := os.Hostname()
	return fmt.Sprintf("%s:%s:%s", runtime.GOOS, runtime.GOARCH, hostname), nil
}

// Load wczytuje licencję z pliku i weryfikuje podpis oraz powiązanie z HWID.
func Load(path, publicKeyPEM, hwid string) (*License, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var lic License
	if err := json.Unmarshal(data, &lic); err != nil {
		return nil, err
	}
	if lic.HardwareID != hwid {
		return nil, fmt.Errorf("licencja nie jest powiązana z tym urządzeniem (HWID mismatch)")
	}
	if err := verifySignature(lic, publicKeyPEM); err != nil {
		return nil, fmt.Errorf("nieprawidłowy podpis licencji: %w", err)
	}
	return &lic, nil
}

// HasFeature sprawdza czy licencja obejmuje daną funkcję premium.
func (l *License) HasFeature(feature string) bool {
	for _, f := range l.Features {
		if f == feature {
			return true
		}
	}
	return false
}

// ----- pomocnicze (RSA weryfikacja, Windows MachineGuid) -----

// verifySignature weryfikuje podpis licencji kluczem publicznym PEM (RSA + SHA-256).
// Podpis (base64) musi być poprawnym podpisem PKCS#1 v1.5 nad ładunkiem
// "<hwid>|<feature1>,<feature2>...". Zwraca błąd gdy podpis jest nieprawidłowy.
func verifySignature(lic License, publicKeyPEM string) error {
	block, _ := pem.Decode([]byte(publicKeyPEM))
	if block == nil || block.Type != "PUBLIC KEY" {
		return fmt.Errorf("nie można sparsować klucza publicznego PEM")
	}

	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return fmt.Errorf("nieprawidłowy klucz publiczny: %w", err)
	}

	rsaPub, ok := pub.(*rsa.PublicKey)
	if !ok {
		return fmt.Errorf("klucz publiczny nie jest kluczem RSA")
	}

	payload := []byte(lic.HardwareID + "|" + strings.Join(lic.Features, ","))
	sig, err := base64.StdEncoding.DecodeString(lic.Signature)
	if err != nil {
		return fmt.Errorf("nieprawidłowy podpis (base64): %w", err)
	}

	hash := sha256.Sum256(payload)
	if err := rsa.VerifyPKCS1v15(rsaPub, crypto.SHA256, hash[:], sig); err != nil {
		return fmt.Errorf("podpis nie zgadza się z ładunkiem licencji: %w", err)
	}
	return nil
}

// windowsMachineGuid odczytuje MachineGuid z rejestru Windows (spójnie z klientem C#).
func windowsMachineGuid() (string, error) {
	out, err := exec.Command("reg", "query",
		`HKEY_LOCAL_MACHINE\SOFTWARE\Microsoft\Cryptography`, "/v", "MachineGuid").Output()
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.Contains(line, "MachineGuid") {
			parts := strings.Fields(line)
			if len(parts) >= 3 {
				return parts[len(parts)-1], nil
			}
		}
	}
	return "", fmt.Errorf("MachineGuid nie znaleziony w rejestrze")
}
