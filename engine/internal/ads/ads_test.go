package ads

import (
	"testing"
)

func testConfig() *Config {
	return &Config{
		Banners: []AdUnit{
			{ID: "ban-heavy", Advertiser: "A", Title: "T", Type: "banner", Weight: 100},
			{ID: "ban-light", Advertiser: "B", Title: "T", Type: "banner", Weight: 1},
		},
		Interstitials: []AdUnit{
			{ID: "int-only", Advertiser: "C", Title: "T", Type: "interstitial", Weight: 5, Seconds: 5},
		},
	}
}

func TestPickBannerReturnsFromList(t *testing.T) {
	c := testConfig()
	for i := 0; i < 100; i++ {
		ad := c.Pick("banner")
		if ad == nil {
			t.Fatalf("Pick(banner) zwrócił nil mimo niepustej listy")
		}
		if ad.ID != "ban-heavy" && ad.ID != "ban-light" {
			t.Errorf("Pick(banner) zwrócił nieznany reklamę: %s", ad.ID)
		}
	}
}

func TestPickInterstitialReturnsFromList(t *testing.T) {
	c := testConfig()
	for i := 0; i < 100; i++ {
		ad := c.Pick("interstitial")
		if ad == nil {
			t.Fatalf("Pick(interstitial) zwrócił nil mimo niepustej listy")
		}
		if ad.ID != "int-only" {
			t.Errorf("Pick(interstitial) zwrócił nieznaną reklamę: %s", ad.ID)
		}
	}
}

func TestPickUnknownTypeReturnsNil(t *testing.T) {
	c := testConfig()
	if ad := c.Pick("nieznany"); ad != nil {
		t.Errorf("Pick(nieznany) powinno zwrócić nil, dostało: %+v", ad)
	}
}

func TestPickEmptyListReturnsNil(t *testing.T) {
	c := &Config{}
	if ad := c.Pick("banner"); ad != nil {
		t.Errorf("Pick(banner) na pustej liście powinno zwrócić nil, dostało: %+v", ad)
	}
}

func TestPickRespectsWeights(t *testing.T) {
	c := testConfig()
	counts := map[string]int{}
	n := 10000
	for i := 0; i < n; i++ {
		ad := c.Pick("banner")
		counts[ad.ID]++
	}
	// ban-heavy ma wagę 100, ban-light wagę 1 → heavy powinien dominować (>99% Auswahl).
	if float64(counts["ban-heavy"])/float64(n) < 0.98 {
		t.Errorf("wagi nie są respektowane: ban-heavy=%d/%d (oczekiwano >98%%)", counts["ban-heavy"], n)
	}
	if counts["ban-light"] == 0 {
		t.Errorf("ban-light (waga 1) nie pojawiło się ani razu w %d losowaniach", n)
	}
}
