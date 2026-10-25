package api

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"social-downloader-engine/internal/ads"
)

// ---------------------------------------------------------------------------
// Panel webowy do zarządzania reklamodawcami (ads-config.json + kreatywy).
//
// Endpointy (WYŁĄCZNIE tryb lokalny, patrz s.adsGuard):
//   GET  /ads-panel     — strona panelu (HTML+JS, zero zależności zewnętrznych)
//   GET  /ads-config    — aktualna konfiguracja (banners + interstitials)
//   POST /ads-config    — walidacja → zapis pliku (backup .bak) → hot-swap
//   POST /ads-image     — upload kreatywy (base64 PNG/JPG/WEBP)
//   GET  /adsimg?file=  — podgląd kreatywy z katalogu obrazów
//
// Bezpieczeństwo:
//  - panel działa tylko, gdy silnik nasłuchuje na loopbacku (bez -addr);
//    w trybie LAN wszystkie endpointy panelu zwracają 403 — end-user nie
//    edytuje reklam, a exposowanie edytora konfiguracji do sieci to zły pomysł,
//  - POST-y wymagają nagłówka Host będącego loopbackiem (DNS rebinding) oraz
//    Content-Type: application/json (klasyczny CSRF z przeglądarki nie wyśle
//    takiego content-type bez preflight, a CORS-jądro serwera nie odpowiada).
// ---------------------------------------------------------------------------

const (
	// maxAdsConfigBody — limit POST /ads-config (razem z JSON overhead).
	maxAdsConfigBody = 256 << 10
	// maxAdsImageUploadBytes — limit decoded kreatywy po base64.
	maxAdsImageUploadBytes = 4 << 20
	// maxAdsPerType — sanity: maks. pozycji jednego typu (banery/interstitials).
	maxAdsPerType = 60
)

// adsImageMime — whitelist typów kreatyw (rozszerzenie → MIME).
var adsImageMime = map[string]string{
	".png":  "image/png",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".webp": "image/webp",
}

// adsNameRe — bezpieczna nazwa pliku kreatywy; blokuje katalogi, dwukropki
// i sterujące — finalna ścieżka to zawsze imagesDir + nazwa.
var adsNameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.\-]{0,63}$`)

// adsIDRe — identyfikator pozycji (id w JSON kontraktu).
var adsIDRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.\-]{0,63}$`)

// hostPart wycina host z "host:port" (obsługuje "[::1]:8765").
func hostPart(addr string) string {
	if i := strings.LastIndex(addr, ":"); i >= 0 {
		addr = addr[:i]
	}
	return strings.ToLower(strings.Trim(addr, "[]"))
}

// hostIsLoopback — true dla 127.0.0.1 / localhost / ::1.
func hostIsLoopback(h string) bool {
	h = hostPart(h)
	return h == "127.0.0.1" || h == "localhost" || h == "::1"
}

// adsGuard weryfikuje warunki dostępu panelu (tryb lokalny + loopback Host).
// Zwraca false i wypisuje 403, gdy coś nie gra. GET/POST wszędzie.
func (s *Server) adsGuard(w http.ResponseWriter, r *http.Request) bool {
	if !s.adsLoopbackOnly() {
		http.Error(w, "Panel reklam działa tylko w trybie lokalnym (uruchom silnik bez -addr).", http.StatusForbidden)
		return false
	}
	if !hostIsLoopback(r.Host) {
		http.Error(w, "Panel dostępny tylko z localhost.", http.StatusForbidden)
		return false
	}
	return true
}

// jsonContentType — wymuszenie application/json (blokada klasycznego CSRF:
// przeglądarka nie może wysłać takiego Content-Type bez preflight).
func jsonContentType(r *http.Request) bool {
	return strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "application/json")
}

// adsLoopbackOnly — panel dostępny tylko, gdy silnik nasłuchuje na loopbacku
// (bez -addr; w trybie LAN endpointy panelu zwracają 403).
func (s *Server) adsLoopbackOnly() bool {
	return hostIsLoopback(s.cfg.Addr)
}

// handleAdsPanel — strona panelu (jedyny plik HTML, brak zależności).
func (s *Server) handleAdsPanel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.adsGuard(w, r) {
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	io.WriteString(w, adsPanelHTML)
}

// currentAds zwraca wskaźnik aktualnej konfiguracji pod adsMu (hot-swap-safe).
func (s *Server) currentAds() *ads.Config {
	s.adsMu.Lock()
	defer s.adsMu.Unlock()
	return s.cfg.Ads
}

// setAds podmienia konfigurację w pamięci (Pick korzysta natychmiast).
func (s *Server) setAds(c *ads.Config) {
	s.adsMu.Lock()
	s.cfg.Ads = c
	s.adsMu.Unlock()
}

// handleAdsConfig: GET — odczyt konfiguracji; POST — walidacja, zapis do
// pliku (z backupem ads-config.json.bak) i podmiana w pamięci (hot-swap).
func (s *Server) handleAdsConfig(w http.ResponseWriter, r *http.Request) {
	if !s.adsGuard(w, r) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		ac := s.currentAds()
		banners := []ads.AdUnit{}
		inters := []ads.AdUnit{}
		if ac != nil {
			banners = ac.Banners
			inters = ac.Interstitials
		}
		writeJSON(w, map[string]interface{}{
			"premium":       s.cfg.License != nil,
			"banners":       banners,
			"interstitials": inters,
			"file":          s.cfg.AdsFile,
		})

	case http.MethodPost:
		if s.cfg.AdsFile == "" {
			http.Error(w, "silnik nie ma skonfigurowanej ścieżki ads-config.json", http.StatusInternalServerError)
			return
		}
		if !jsonContentType(r) {
			http.Error(w, "wymagany Content-Type: application/json", http.StatusUnsupportedMediaType)
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, maxAdsConfigBody+1024))
		if err != nil {
			http.Error(w, "błąd odczytu body", http.StatusBadRequest)
			return
		}
		if len(body) > maxAdsConfigBody {
			http.Error(w, "konfiguracja zbyt duża", http.StatusRequestEntityTooLarge)
			return
		}
		var req struct {
			Banners       []ads.AdUnit `json:"banners"`
			Interstitials []ads.AdUnit `json:"interstitials"`
		}
		if err := json.Unmarshal(body, &req); err != nil {
			http.Error(w, "nieprawidłowy JSON: "+err.Error(), http.StatusBadRequest)
			return
		}
		cfg := &ads.Config{Banners: req.Banners, Interstitials: req.Interstitials}
		warnings, verr := s.validateAdConfig(cfg)
		if verr != nil {
			http.Error(w, verr.Error(), http.StatusBadRequest)
			return
		}
		if err := saveAdsConfigFile(s.cfg.AdsFile, cfg); err != nil {
			http.Error(w, "zapis pliku nieudany: "+err.Error(), http.StatusInternalServerError)
			return
		}
		s.setAds(cfg)
		log.Printf("[ads-panel] konfiguracja zapisana: %d banerów, %d interstitialów (%s)",
			len(cfg.Banners), len(cfg.Interstitials), s.cfg.AdsFile)
		writeJSON(w, map[string]interface{}{
			"ok":            true,
			"warnings":      warnings,
			"banners":       cfg.Banners,
			"interstitials": cfg.Interstitials,
		})

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// validateAdConfig — walidacja fail-closed + ostrzeżenia nietywarowe
// (np. obrazek nie istnieje na dysku — zapis przechodzi, autor widzi ostrzeżenie).
func (s *Server) validateAdConfig(c *ads.Config) ([]string, error) {
	warnings := []string{}
	seen := map[string]bool{}

	check := func(list []ads.AdUnit, typ string) error {
		if len(list) > maxAdsPerType {
			return fmt.Errorf("%s: zbyt wiele pozycji (max %d)", typ, maxAdsPerType)
		}
		for i := range list {
			a := &list[i]
			a.ID = strings.TrimSpace(a.ID)
			a.Advertiser = strings.TrimSpace(a.Advertiser)
			a.Title = strings.TrimSpace(a.Title)
			a.Description = strings.TrimSpace(a.Description)
			a.URL = strings.TrimSpace(a.URL)
			a.Image = strings.TrimSpace(strings.ReplaceAll(a.Image, "\\", "/"))
			a.Clickthrough = strings.TrimSpace(a.Clickthrough)

			if !adsIDRe.MatchString(a.ID) {
				return fmt.Errorf("%s[%d]: nieprawidłowy id %q (litery, cyfry, _ . -)", typ, i+1, a.ID)
			}
			if seen[a.ID] {
				return fmt.Errorf("%s[%d]: duplikat id %q", typ, i+1, a.ID)
			}
			seen[a.ID] = true

			if a.Advertiser == "" || len(a.Advertiser) > 64 {
				return fmt.Errorf("%s[%d] (%s): reklamodawca wymagany, max 64 znaki", typ, i+1, a.ID)
			}
			if a.Title == "" || len(a.Title) > 120 {
				return fmt.Errorf("%s[%d] (%s): tytuł wymagany, max 120 znaków", typ, i+1, a.ID)
			}
			if len(a.Description) > 240 {
				return fmt.Errorf("%s[%d] (%s): opis max 240 znaków", typ, i+1, a.ID)
			}
			if err := validateURL(a.URL); err != nil {
				return fmt.Errorf("%s[%d] (%s): url — %v", typ, i+1, a.ID, err)
			}
			if a.Weight < 1 || a.Weight > 1000 {
				return fmt.Errorf("%s[%d] (%s): waga w zakresie 1..1000 (jest %d)", typ, i+1, a.ID, a.Weight)
			}
			if typ == "interstitial" {
				if a.Seconds < 1 || a.Seconds > 60 {
					return fmt.Errorf("%s[%d] (%s): interstitial seconds w zakresie 1..60 (jest %d)", typ, i+1, a.ID, a.Seconds)
				}
			}
			if a.Clickthrough != "" && hasCtl(a.Clickthrough) {
				return fmt.Errorf("%s[%d] (%s): clickthrough — znaki sterujące", typ, i+1, a.ID)
			}
			if hasCtl(a.URL) || hasCtl(a.Image) {
				return fmt.Errorf("%s[%d] (%s): znaki sterujące w url/image", typ, i+1, a.ID)
			}

			// image: goły URL http(s) albo ścieżka "adsimg/<plik>".
			if a.Image != "" {
				low := strings.ToLower(a.Image)
				if !(strings.HasPrefix(low, "http://") || strings.HasPrefix(low, "https://")) {
					if err := validImageRelPath(a.Image); err != nil {
						return fmt.Errorf("%s[%d] (%s): %v", typ, i+1, a.ID, err)
					}
					name := filepath.Base(a.Image)
					if s.findAdsImageDir(name) == "" {
						warnings = append(warnings, fmt.Sprintf(
							"%s (%s): obraz %s nie istnieje na dysku (wgryj przez panel lub popraw ścieżkę)", a.ID, a.Advertiser, a.Image))
					}
				}
			}
		}
		return nil
	}

	if err := check(c.Banners, "banner"); err != nil {
		return warnings, err
	}
	if err := check(c.Interstitials, "interstitial"); err != nil {
		return warnings, err
	}
	return warnings, nil
}

// validImageRelPath — dopuszcza wyłącznie "adsimg/<bezpieczna-nazwa>".
// Blokuje: katalogi-poza-adsimg, "..", backslashe, dwukropki (Windows),
// dziwne rozszerzenia. Kreatywy http zewnątrzne nie wchodzą tu.
func validImageRelPath(p string) error {
	low := strings.ToLower(p)
	if loc := strings.Index(low, ".."); loc >= 0 {
		return errors.New("ścieżka obrazu nie może zawierać '..'")
	}
	if strings.ContainsAny(p, "\\/:") && !strings.HasPrefix(low, "adsimg/") {
		return errors.New("ścieżka obrazu musi zaczynać się od adsimg/")
	}
	if !strings.HasPrefix(low, "adsimg/") {
		return fmt.Errorf("ścieżka obrazu musi zaczynać się od adsimg/ (jest %q)", p)
	}
	name := p[len("adsimg/"):]
	ext := strings.ToLower(filepath.Ext(name))
	if _, ok := adsImageMime[ext]; !ok {
		return fmt.Errorf("rozszerzenie obrazu dozwolone: png/jpg/webp (jest %q)", ext)
	}
	if !adsNameRe.MatchString(name) {
		return fmt.Errorf("nieprawidłowa nazwa pliku obrazu %q", name)
	}
	return nil
}

// hasCtl — znaki sterujące (<0x20) w treściach tekstowych.
func hasCtl(s string) bool {
	for _, r := range s {
		if r < 0x20 {
			return true
		}
	}
	return false
}

// findAdsImageDir zwraca katalog, w którym istnieje <name> ("" gdy brak).
func (s *Server) findAdsImageDir(name string) string {
	for _, d := range s.cfg.AdsImageDirs {
		if d == "" {
			continue
		}
		if st, err := os.Stat(filepath.Join(d, name)); err == nil && !st.IsDir() {
			return d
		}
	}
	return ""
}

// adsUploadDir — katalog docelowy uploadu: pierwszy istniejący kandydat
// (to folder, w którym klient też szuka obrazów); gdy nic nie istnieje,
// tworzymy katalog engineDir/adsimg (kandydat 0).
func (s *Server) adsUploadDir() (string, error) {
	var first = ""
	if len(s.cfg.AdsImageDirs) > 0 {
		first = s.cfg.AdsImageDirs[0]
	}
	for _, d := range s.cfg.AdsImageDirs {
		if d == "" {
			continue
		}
		if st, err := os.Stat(d); err == nil && st.IsDir() {
			return d, nil
		}
	}
	if first == "" {
		return "", errors.New("brak skonfigurowanego katalogu obrazów")
	}
	if err := os.MkdirAll(first, 0o755); err != nil {
		return "", err
	}
	return first, nil
}

// uniqueImageName banner_x.png istnieje → banner_x_2.png (i tak dalej).
func uniqueImageName(dir, name string) string {
	if _, err := os.Stat(filepath.Join(dir, name)); os.IsNotExist(err) {
		return name
	}
	ext := strings.ToLower(filepath.Ext(name))
	base := strings.TrimSuffix(name, filepath.Ext(name))
	for i := 2; i < 100; i++ {
		cand := fmt.Sprintf("%s_%d%s", base, i, ext)
		if _, err := os.Stat(filepath.Join(dir, cand)); os.IsNotExist(err) {
			return cand
		}
	}
	return fmt.Sprintf("%s_%d%s", base, time.Now().UnixNano(), ext)
}

// handleAdsImage — upload pojedynczej kreatywy: {name, data(base64)}.
// Magic bytes walidowane na dysku (nie ufamy rozszerzeniu od przeglądarki).
func (s *Server) handleAdsImage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.adsGuard(w, r) {
		return
	}
	if !jsonContentType(r) {
		http.Error(w, "wymagany Content-Type: application/json", http.StatusUnsupportedMediaType)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 8<<20))
	if err != nil {
		http.Error(w, "błąd odczytu body", http.StatusBadRequest)
		return
	}
	var req struct {
		Name string `json:"name"`
		Data string `json:"data"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "nieprawidłowy JSON", http.StatusBadRequest)
		return
	}
	ext := strings.ToLower(filepath.Ext(strings.TrimSpace(req.Name)))
	if _, ok := adsImageMime[ext]; !ok {
		http.Error(w, "dozwolone obrazy: .png/.jpg/.webp", http.StatusBadRequest)
		return
	}
	if !adsNameRe.MatchString(strings.TrimSpace(req.Name)) {
		http.Error(w, "nieprawidłowa nazwa pliku (litery, cyfry, _ . -)", http.StatusBadRequest)
		return
	}
	data, err := base64.StdEncoding.DecodeString(req.Data)
	if err != nil {
		http.Error(w, "błąd dekodowania base64", http.StatusBadRequest)
		return
	}
	if len(data) > maxAdsImageUploadBytes {
		http.Error(w, "obraz zbyt duży (max 4 MB)", http.StatusRequestEntityTooLarge)
		return
	}
	if !imageMagicOK(data, ext) {
		http.Error(w, "plik nie wygląda na obraz "+strings.TrimPrefix(ext, "."), http.StatusBadRequest)
		return
	}
	dir, err := s.adsUploadDir()
	if err != nil {
		http.Error(w, "katalog obrazów niedostępny: "+err.Error(), http.StatusInternalServerError)
		return
	}
	name := uniqueImageName(dir, strings.TrimSpace(req.Name))
	if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
		http.Error(w, "zapis obrazu nieudany: "+err.Error(), http.StatusInternalServerError)
		return
	}
	log.Printf("[ads-panel] obraz zapisany: %s (%d B)", filepath.Join(dir, name), len(data))
	writeJSON(w, map[string]interface{}{
		"ok":   true,
		"path": "adsimg/" + name,
		"size": len(data),
	})
}

// imageMagicOK — nagłówek pliku zgodny z deklarowanym typem (png/jpg/webp).
func imageMagicOK(data []byte, ext string) bool {
	switch ext {
	case ".png":
		return len(data) >= 8 && string(data[:8]) == "\x89PNG\r\n\x1a\n"
	case ".jpg", ".jpeg":
		return len(data) >= 3 && data[0] == 0xFF && data[1] == 0xD8 && data[2] == 0xFF
	case ".webp":
		return len(data) >= 12 && string(data[:4]) == "RIFF" && string(data[8:12]) == "WEBP"
	}
	return false
}

// handleAdsImg — podgląd kreatywy: GET /adsimg?file=banner_omega.png.
// Szukamy w każdym katalogu z AdsImageDirs (kolejność kandydatów).
func (s *Server) handleAdsImg(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.adsGuard(w, r) {
		return
	}
	file := filepath.Base(strings.TrimSpace(r.URL.Query().Get("file")))
	if file == "." || file == "/" {
		http.Error(w, "brak parametru file", http.StatusBadRequest)
		return
	}
	ext := strings.ToLower(filepath.Ext(file))
	if _, ok := adsImageMime[ext]; !ok {
		http.Error(w, "nieobsługiwany typ pliku", http.StatusBadRequest)
		return
	}
	if !adsNameRe.MatchString(file) {
		http.Error(w, "nieprawidłowa nazwa pliku", http.StatusBadRequest)
		return
	}
	dir := s.findAdsImageDir(file)
	if dir == "" {
		http.Error(w, "obraz nie znaleziony", http.StatusNotFound)
		return
	}
	path := filepath.Join(dir, file)
	fi, err := os.Stat(path)
	if err != nil {
		http.Error(w, "obraz nie znaleziony", http.StatusNotFound)
		return
	}
	f, err := os.Open(path)
	if err != nil {
		http.Error(w, "nie można otworzyć obrazu", http.StatusInternalServerError)
		return
	}
	defer f.Close()
	w.Header().Set("Content-Type", adsImageMime[ext])
	http.ServeContent(w, r, file, fi.ModTime(), f)
}

// saveAdsConfigFile — atomowy-ish zapis konfiguracji:
//  1. json.MarshalIndent → ads-config.json.tmp,
//  2. backup bieżącej treści → ads-config.json.bak (best effort),
//  3. remove+rename (Windows nie nadpisuje rename na istniejącym pliku).
func saveAdsConfigFile(path string, cfg *ads.Config) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	if cur, rerr := os.ReadFile(path); rerr == nil {
		_ = os.WriteFile(path+".bak", cur, 0o644)
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	return nil
}
