package ads

import (
	"encoding/json"
	"math/rand"
	"os"
)

// AdUnit to pojedyncza reklama (baner lub interstitial).
type AdUnit struct {
	ID           string `json:"id"`
	Advertiser   string `json:"advertiser"`
	Title        string `json:"title"`
	Description  string `json:"description"`
	URL          string `json:"url"`          // link docelowy po kliknięciu
	Image        string `json:"image"`        // ścieżka lokalna lub URL do grafiki
	Type         string `json:"type"`         // banner | interstitial
	Weight       int    `json:"weight"`       // waga w losowaniu (domyślnie 1)
	Seconds      int    `json:"seconds"`      // czas wyświetlania interstitial (sekundy)
	Clickthrough string `json:"clickthrough"` // parametr śledzenia kliknięcia (opcjonalnie)
}

// Config to cała konfiguracja reklam wczytywana z pliku JSON.
type Config struct {
	Banners       []AdUnit `json:"banners"`
	Interstitials []AdUnit `json:"interstitials"`
}

// Load wczytuje konfigurację reklam z pliku.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var c Config
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, err
	}
	// ustaw domyślne wagi
	for i := range c.Banners {
		if c.Banners[i].Weight <= 0 {
			c.Banners[i].Weight = 1
		}
	}
	for i := range c.Interstitials {
		if c.Interstitials[i].Weight <= 0 {
			c.Interstitials[i].Weight = 1
		}
	}
	return &c, nil
}

// Pick losowo wybiera reklamę danego typu z uwzględnieniem wag.
func (c *Config) Pick(typ string) *AdUnit {
	var list []AdUnit
	switch typ {
	case "banner":
		list = c.Banners
	case "interstitial":
		list = c.Interstitials
	default:
		return nil
	}
	if len(list) == 0 {
		return nil
	}

	// losowanie wagowane
	total := 0
	for _, a := range list {
		total += a.Weight
	}
	r := rand.Intn(total)
	for _, a := range list {
		r -= a.Weight
		if r < 0 {
			return &a
		}
	}
	return &list[0]
}

// Pick losowo wybiera reklamę danego typu z uwzględnieniem wag.
