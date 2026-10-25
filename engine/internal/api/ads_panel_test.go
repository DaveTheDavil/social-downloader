package api

import (
	"encoding/base64"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"social-downloader-engine/internal/ads"
)

// panelServer — serwer panelu w trybie lokalnym: config+obrazy w temp-dir.
func panelServer(t *testing.T) (*Server, string, string) {
	t.Helper()
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "ads-config.json")
	imgDir := filepath.Join(tmp, "AdsImg")
	if err := os.MkdirAll(imgDir, 0o755); err != nil {
		t.Fatalf("temp dir: %v", err)
	}
	if err := os.WriteFile(cfgPath, []byte("{}"), 0o644); err != nil {
		t.Fatalf("seed config: %v", err)
	}
	s := New(&Config{
		Addr:         "127.0.0.1:8765",
		Ads:          &ads.Config{},
		AdsFile:      cfgPath,
		AdsImageDirs: []string{imgDir},
	})
	return s, cfgPath, imgDir
}

// panelReq — request przez pełny router (mux), z kontrolą Host.
func panelReq(s *Server, method, target, host, ctype, body string) *httptest.ResponseRecorder {
	var bd *strings.Reader
	if body == "" {
		bd = strings.NewReader("")
	} else {
		bd = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, target, bd)
	req.Host = host
	if ctype != "" {
		req.Header.Set("Content-Type", ctype)
	}
	w := httptest.NewRecorder()
	s.Router().ServeHTTP(w, req)
	return w
}

func validPayload() string {
	return `{"banners":[{"id":"ban-1","advertiser":"Acme","title":"Tytul","description":"Opis","url":"https://example.com/","image":"adsimg/ban.png","weight":10,"type":"banner"}],` +
		`"interstitials":[{"id":"int-1","advertiser":"Acme","title":"Tytul","url":"https://example.com/","image":"adsimg/int.png","weight":5,"seconds":5,"type":"interstitial"}]}`
}

func TestAdsPanelHTMLServed(t *testing.T) {
	s, _, _ := panelServer(t)
	w := panelReq(s, "GET", "/ads-panel", "127.0.0.1:8765", "", "")
	if w.Code != 200 {
		t.Fatalf("status %d, oczekiwano 200", w.Code)
	}
	if !strings.Contains(w.Body.String(), "Panel reklamodawców") {
		t.Error("strona panelu bez oczekiwanego nagłówka")
	}
}

func TestAdsConfigPostPersistsAndSwaps(t *testing.T) {
	s, cfgPath, _ := panelServer(t)
	w := panelReq(s, "POST", "/ads-config", "127.0.0.1:8765", "application/json", validPayload())
	if w.Code != 200 {
		t.Fatalf("POST /ads-config: status %d, body=%s", w.Code, w.Body.String())
	}
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("plik konfiguracji nie powstał: %v", err)
	}
	if !strings.Contains(string(data), "Acme") {
		t.Error("zapisany plik nie zawiera reklamodawcy")
	}

	// backup .bak powstał z wcześniejszej treści "{}"
	bak, err := os.ReadFile(cfgPath + ".bak")
	if err != nil || strings.TrimSpace(string(bak)) != "{}" {
		t.Error("backup .bak brak lub ma złą treść (po pierwszym zapisie)")
	}

	// hot-swap: GET /ads:type=banner od razu serwuje nową pozycję.
	w2 := panelReq(s, "GET", "/ads?type=banner", "127.0.0.1:8765", "", "")
	var adResp struct {
		Ad map[string]interface{} `json:"ad"`
	}
	if err := json.Unmarshal(w2.Body.Bytes(), &adResp); err != nil {
		t.Fatalf("GET /ads: %v", err)
	}
	if adResp.Ad == nil || adResp.Ad["id"] != "ban-1" {
		t.Errorf("hot-swap nie działa: ad=%v", adResp.Ad)
	}

	// GET /ads-config zwraca odczyt z pamięci
	w3 := panelReq(s, "GET", "/ads-config", "127.0.0.1:8765", "", "")
	var cfg struct {
		Banners []ads.AdUnit `json:"banners"`
		File    string       `json:"file"`
	}
	if err := json.Unmarshal(w3.Body.Bytes(), &cfg); err != nil {
		t.Fatalf("GET /ads-config: %v", err)
	}
	if len(cfg.Banners) != 1 || cfg.Banners[0].Advertiser != "Acme" {
		t.Errorf("GET /ads-config: %+v", cfg.Banners)
	}
	if cfg.File == "" {
		t.Error("GET /ads-config bez ścieżki pliku")
	}
}

func TestAdsConfigPostValidationErrors(t *testing.T) {
	s, _, _ := panelServer(t)
	cases := []struct {
		name string
		body string
		want string
	}{
		{"waga 0", `{"banners":[{"id":"a","advertiser":"X","title":"T","url":"https://e.com/","weight":0}]}`, "waga"},
		{"duplikat id", `{"banners":[{"id":"a","advertiser":"X","title":"T","url":"https://e.com/","weight":1},{"id":"a","advertiser":"Y","title":"T","url":"https://e.com/","weight":1}]}`, "duplikat"},
		{"zły url", `{"banners":[{"id":"a","advertiser":"X","title":"T","url":"ftp://e.com/","weight":1}]}`, "url"},
		{"traversal w obrazie", `{"banners":[{"id":"a","advertiser":"X","title":"T","url":"https://e.com/","image":"adsimg/../x.png","weight":1}]}`, "obraz"},
		{"pusty tytuł", `{"banners":[{"id":"a","advertiser":"X","title":"","url":"https://e.com/","weight":1}]}`, "tytuł"},
		{"cj interstitial", `{"interstitials":[{"id":"a","advertiser":"X","title":"T","url":"https://e.com/","weight":1,"seconds":0}]}`, "seconds"},
		{"za dużo pozycji", "", "zbyt wiele"},
	}
	buf := `{"banners":[`
	for i := 0; i < maxAdsPerType+1; i++ {
		if i > 0 {
			buf += ","
		}
		buf += `{"id":"b` + itoaa(i) + `","advertiser":"X","title":"T","url":"https://e.com/","weight":1}`
	}
	buf += "]}"
	cases[len(cases)-1].body = buf

	for _, c := range cases {
		w := panelReq(s, "POST", "/ads-config", "127.0.0.1:8765", "application/json", c.body)
		if w.Code != 400 {
			t.Errorf("%s: status %d (oczekiwano 400), body=%s", c.name, w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), c.want) {
			t.Errorf("%s: komunikat bez %q: %s", c.name, c.want, w.Body.String())
		}
	}
}

func itoaa(i int) string {
	return string(rune('a'+i%26)) + string(rune('0'+i/26%10)) + string(rune('0'+i/260%10))
}

func TestAdsConfigPostGuards(t *testing.T) {
	s, _, _ := panelServer(t)

	// niewłaściwy Content-Type → 415
	w := panelReq(s, "POST", "/ads-config", "127.0.0.1:8765", "text/plain", validPayload())
	if w.Code != 415 {
		t.Errorf("Content-Type text/plain: %d (oczekiwano 415)", w.Code)
	}

	// Host spoza loopback → 403 (w tym DNS-rebinding)
	w2 := panelReq(s, "POST", "/ads-config", "evil.example.com", "application/json", validPayload())
	if w2.Code != 403 {
		t.Errorf("obcy Host: %d (oczekiwano 403)", w2.Code)
	}

	// Tryb LAN (0.0.0.0) — panel całkowicie zablokowany.
	tmp := t.TempDir()
	s2 := New(&Config{
		Addr: "0.0.0.0:8765", Ads: &ads.Config{},
		AdsFile: filepath.Join(tmp, "c.json"), AdsImageDirs: []string{tmp},
	})
	w3 := panelReq(s2, "GET", "/ads-panel", "192.168.0.5:8765", "", "")
	if w3.Code != 403 {
		t.Errorf("panel w LAN: %d (oczekiwano 403)", w3.Code)
	}
	w4 := panelReq(s2, "POST", "/ads-config", "192.168.0.5:8765", "application/json", validPayload())
	if w4.Code != 403 {
		t.Errorf("POST config w LAN: %d (oczekiwano 403)", w4.Code)
	}
}

func TestAdsImageUploadFlow(t *testing.T) {
	s, _, imgDir := panelServer(t)
	pngPh := "\x89PNG\r\n\x1a\n" + strings.Repeat("payload", 20)
	b64 := base64.StdEncoding.EncodeToString([]byte(pngPh))

	// 1) poprawny upload (nazwa LAN: sanitizacja robi się po stronie panelu)
	body := `{"name":"kreatywa-test.png","data":"` + b64 + `"}`
	w := panelReq(s, "POST", "/ads-image", "127.0.0.1:8765", "application/json", body)
	if w.Code != 200 {
		t.Fatalf("upload: %d body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Ok   bool   `json:"ok"`
		Path string `json:"path"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("upload JSON: %v", err)
	}
	if !resp.Ok || resp.Path != "adsimg/kreatywa-test.png" {
		t.Errorf("upload: ok=%v path=%q (oczekiwano adsimg/kreatywa-test.png)", resp.Ok, resp.Path)
	}
	if _, err := os.Stat(filepath.Join(imgDir, "kreatywa-test.png")); err != nil {
		t.Errorf("plik na dysku brak: %v", err)
	}

	// 2) re-upload tej samej nazwy → unikalny sufiks
	w2 := panelReq(s, "POST", "/ads-image", "127.0.0.1:8765", "application/json", body)
	var resp2 struct {
		Path string `json:"path"`
	}
	_ = json.Unmarshal(w2.Body.Bytes(), &resp2)
	if resp2.Path == resp.Path || !strings.Contains(resp2.Path, "_2") {
		t.Errorf("kolizja nazwy nie rozwiązana: %q", resp2.Path)
	}

	// 3) złe magic bytes
	bad := `{"name":"x.png","data":"` + base64.StdEncoding.EncodeToString([]byte("definitely-not-a-png")) + `"}`
	w3 := panelReq(s, "POST", "/ads-image", "127.0.0.1:8765", "application/json", bad)
	if w3.Code != 400 || !strings.Contains(w3.Body.String(), "nie wygląda") {
		t.Errorf("zły magic: %d %s", w3.Code, w3.Body.String())
	}

	// 4) niedozwolone rozszerzenie
	badExt := `{"name":"x.gif","data":"` + b64 + `"}`
	w4 := panelReq(s, "POST", "/ads-image", "127.0.0.1:8765", "application/json", badExt)
	if w4.Code != 400 {
		t.Errorf("rozszerzenie .gif: %d (oczekiwano 400)", w4.Code)
	}

	// 5) PUT jest niedozwolony (tylko POST)
	w5 := panelReq(s, "GET", "/adsimg?file=kreatywa-test.png", "127.0.0.1:8765", "", "")
	if w5.Code != 200 {
		t.Errorf("podgląd obrazu: %d", w5.Code)
	}
	w6 := panelReq(s, "GET", "/adsimg?file=../sekrat.png", "127.0.0.1:8765", "", "")
	if w6.Code == 200 {
		t.Errorf("traversal w /adsimg przechodzi?! : %d", w6.Code)
	}
	w7 := panelReq(s, "GET", "/adsimg?file=brak.png", "127.0.0.1:8765", "", "")
	if w7.Code != 404 {
		t.Errorf("nieistniejący obraz: %d (oczekiwano 404)", w7.Code)
	}
}

func TestValidateWarningsForMissingImage(t *testing.T) {
	s, _, _ := panelServer(t)
	// baner z obrazem, którego nie ma na dysku → zapis OK + ostrzeżenie
	body := `{"banners":[{"id":"a","advertiser":"X","title":"T","url":"https://e.com/","image":"adsimg/niema.png","weight":1}],"interstitials":[]}`
	w := panelReq(s, "POST", "/ads-config", "127.0.0.1:8765", "application/json", body)
	if w.Code != 200 {
		t.Fatalf("status %d body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Warnings []string `json:"warnings"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if len(resp.Warnings) != 1 || !strings.Contains(resp.Warnings[0], "niema.png") {
		t.Errorf("brak ostrzeżenia o brakującym obrazie: %+v", resp.Warnings)
	}
}
