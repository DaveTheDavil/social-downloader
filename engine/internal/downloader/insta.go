package downloader

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// ErrNoVideoFormats — yt-dlp nie znalazł formatów WIDEO. Typowe dla postów IG
// ze zdjęciami: yt-dlp obsługuje Instagram tylko dla wideo/reels, a posty
// ze zdjęciami kończą się "No video formats found!". Taki błąd uruchamia
// w api fallback na anonimowy resolver obrazu (ResolveInstagramPhoto).
var ErrNoVideoFormats = errors.New("post nie zawiera wideo (brak formatów w video)")

// browserUA — UA przeglądarki dla bezpośrednich żądań HTTP. Instagram bez
// przeglądarkowego UA przekierowuje na login nawet stary redirect /media/.
const browserUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"

// instagramRe — kod posta/reelsa w URL Instagrama (instagr.am obejmuje skróty).
var instagramRe = regexp.MustCompile(`(?i)instagram\.(?:com|am)/(?:p|reel|tv)/([A-Za-z0-9_-]+)`)

// InstagramShortcode zwraca kod posta IG z URL (np. "DexBwdADpTo") lub "".
func InstagramShortcode(rawURL string) string {
	m := instagramRe.FindStringSubmatch(rawURL)
	if m == nil {
		return ""
	}
	return m[1]
}

// ResolveInstagramPhoto rozwiązuje post IG ze ZDJĘCIEM przez publiczny,
// anonimowy redirect https://www.instagram.com/p/<kod>/media/?size=l →
// plik na CDN (zweryfikowane 2026-10-23; pojedynczy obraz i PIERWSZY element
// serii — pełna lista serii wymaga sesji IG i jest planowana).
// Zwraca syntetyczne metadane (jeden format "img") i finalny URL pliku.
// Fail-closed: /download rozwiązuje plik na nowo, /info nie ujawnia URL.
func ResolveInstagramPhoto(ctx context.Context, shortcode string) (*VideoInfo, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"https://www.instagram.com/p/"+shortcode+"/media/?size=l", nil)
	if err != nil {
		return nil, "", fmt.Errorf("instagram: budowa żądania: %w", err)
	}
	req.Header.Set("User-Agent", browserUA)
	req.Header.Set("Referer", "https://www.instagram.com/")
	req.Header.Set("Accept", "image/avif,image/webp,image/apng,image/*,*/*;q=0.8")

	var finalURL string
	client := &http.Client{
		Timeout: 60 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			finalURL = req.URL.String()
			if len(via) > 5 {
				return errors.New("instagram: zbyt wiele przekierowań")
			}
			return nil
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("instagram: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("instagram: HTTP %d przy pobieraniu zdjęcia", resp.StatusCode)
	}

	ct := strings.ToLower(resp.Header.Get("Content-Type"))
	if !strings.Contains(ct, "image/") {
		return nil, "", fmt.Errorf("instagram: oczekiwano obrazu, otrzymano %q", ct)
	}
	ext := "jpg"
	switch {
	case strings.Contains(ct, "png"):
		ext = "png"
	case strings.Contains(ct, "webp"):
		ext = "webp"
	}

	// Wymiary z nagłówka JPEG (PNG/WebP pomijane — wariant "l" to pełna jakość).
	w, h := 0, 0
	if strings.Contains(ct, "jpeg") {
		w, h, _ = jpegDimensions(resp.Body) // brak wymiarów ≠ błąd rozwiązania
	}

	vi := &VideoInfo{
		ID:       shortcode,
		Title:    "Zdjęcie z Instagrama (" + shortcode + ")",
		Duration: 0,
		Formats: []Format{{
			FormatID:   "img",
			Ext:        ext,
			Width:      w,
			Height:     h,
			VCodec:     "image",
			ACodec:     "none",
			FormatNote: "obraz (pełna jakość)",
		}},
	}
	return vi, finalURL, nil
}

// ErrSessionInvalid — sesyjny resolver IG uznał cookie za nieważne (redirect
// na /accounts/login albo HTTP 401/403). Tekst zawiera „wygasła"/„nieprawidłowa",
// bo warstwa api rozpoznaje ten przypadek także po komunikacie.
var ErrSessionInvalid = errors.New("instagram: sesja IG wygasła lub nieprawidłowa")

// PostItem to jeden element postu IG (zdjęcie lub wideo) z sesyjnego resolvera.
// Index jest 1-based, URL prowadzi wprost do pliku na CDN (bez yt-dlp).
type PostItem struct {
	Index    int    // 1-based
	ID       string // id media/prezentacji w API IG
	Title    string // caption skrót + „ — część N/M" dla serii
	IsImage  bool
	Duration int // sekundy (wideo), 0 dla obrazów
	Width    int
	Height   int
	URL      string // wprost plik (obiekt CDN)
	Ext      string // jpg/png/webp lub mp4
}

// igCandidate — jeden wariant rozmiaru mediów (obraz lub wideo).
type igCandidate struct {
	URL    string `json:"url"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

// igMedia — media z odpowiedzi api/v1/media/<code>/info/ (rekurencyjnie:
// dzieci karuzeli mają tę samą strukturę).
type igMedia struct {
	Code      string `json:"code"`
	ID        string `json:"id"`
	MediaType int    `json:"media_type"` // 1=zdjęcie, 2=wideo, 8=karuzela
	Caption   *struct {
		Text string `json:"text"`
	} `json:"caption"`
	VideoDuration  float64 `json:"video_duration"`
	ImageVersions2 *struct {
		Candidates []igCandidate `json:"candidates"`
	} `json:"image_versions2"`
	VideoVersions []igCandidate `json:"video_versions"`
	CarouselMedia []igMedia     `json:"carousel_media"`
	Owner         *struct {
		Username string `json:"username"`
	} `json:"owner"`
}

// igMediaFeed — koperta odpowiedzi.
type igMediaFeed struct {
	Items []igMedia `json:"items"`
}

// ResolveInstagramPost rozwiązuje post IG przez sesyjne API (cookie użytkownika):
// GET https://www.instagram.com/api/v1/media/<kod>/info/ — pełna lista elementów
// serii karuzeli (media_type 8), pojedyncze zdjęcie (1) i wideo/reels (2).
// Klient NIE podąża za redirectem na /accounts/login → ErrSessionInvalid.
// Formats w VideoInfo pozostają puste — formaty per element buduje warstwa api
// (syntetyczne "img"/"vid" w entries), tu zwracamy wyłącznie PostItem.
func ResolveInstagramPost(ctx context.Context, shortcode, cookie string) (*VideoInfo, []PostItem, error) {
	if strings.TrimSpace(cookie) == "" {
		return nil, nil, fmt.Errorf("%w: brak cookie", ErrSessionInvalid)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"https://www.instagram.com/api/v1/media/"+shortcode+"/info/", nil)
	if err != nil {
		return nil, nil, fmt.Errorf("instagram: budowa żądania: %w", err)
	}
	req.Header.Set("Cookie", cookie)
	req.Header.Set("User-Agent", browserUA)
	req.Header.Set("x-ig-app-id", "936619743392459")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Referer", "https://www.instagram.com/")

	client := &http.Client{
		Timeout: 30 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if strings.Contains(req.URL.Path, "/accounts/login") {
				return ErrSessionInvalid
			}
			if len(via) > 5 {
				return errors.New("instagram: zbyt wiele przekierowań")
			}
			return nil
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		// err z CheckRedirect to nasz sentinel; inne błędy opakuj z kontekstem.
		if errors.Is(err, ErrSessionInvalid) {
			return nil, nil, err
		}
		return nil, nil, fmt.Errorf("instagram: %w", err)
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
		// kontynuuj
	case http.StatusUnauthorized, http.StatusForbidden:
		return nil, nil, fmt.Errorf("%w (HTTP %d)", ErrSessionInvalid, resp.StatusCode)
	default:
		return nil, nil, fmt.Errorf("instagram: HTTP %d przy sesyjnym resolverze", resp.StatusCode)
	}

	var feed igMediaFeed
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&feed); err != nil {
		return nil, nil, fmt.Errorf("instagram: odpowiedź nie jest JSON: %w", err)
	}
	if len(feed.Items) == 0 {
		return nil, nil, errors.New("instagram: odpowiedź bez pozycji")
	}
	root := feed.Items[0]

	uploader := ""
	if root.Owner != nil {
		uploader = root.Owner.Username
	}
	caption := igCaption(root)

	var items []PostItem
	switch root.MediaType {
	case 8: // karuzela
		for i := range root.CarouselMedia {
			child := &root.CarouselMedia[i]
			it, terr := igChildItem(child, i+1, len(root.CarouselMedia), caption)
			if terr != nil {
				return nil, nil, terr
			}
			items = append(items, *it)
		}
	case 1, 2: // pojedyncze zdjęcie lub wideo
		it, terr := igChildItem(&root, 1, 1, caption)
		if terr != nil {
			return nil, nil, terr
		}
		items = append(items, *it)
	default:
		return nil, nil, fmt.Errorf("instagram: nieobsługiwany typ mediów (%d)", root.MediaType)
	}

	title := caption
	if title == "" {
		title = "Instagram (" + shortcode + ")"
	}
	vi := &VideoInfo{
		ID:       shortcode,
		Title:    title,
		Duration: items[0].Duration,
		Uploader: uploader,
	}
	return vi, items, nil
}

// igChildItem buduje PostItem z medii karuzeli/pojedynczego postu (media_type
// 1/2); znacznik „część N/M" tylko dla serii (total > 1).
func igChildItem(m *igMedia, n, total int, caption string) (*PostItem, error) {
	it := &PostItem{Index: n, ID: m.ID, IsImage: m.MediaType == 1}
	if total > 1 {
		suffix := fmt.Sprintf(" — część %d/%d", n, total)
		it.Title = truncCaption(caption, 60) + suffix
	} else {
		it.Title = truncCaption(caption, 60)
	}
	switch m.MediaType {
	case 1: // zdjęcie: NAJWIĘKSZY candidate z image_versions2
		c := igLargest(m.ImageVersions2.Candidates) // nil-safe: nil slice
		if c == nil || c.URL == "" {
			return nil, fmt.Errorf("instagram: pozycja %d bez URL zdjęcia", n)
		}
		it.Width, it.Height, it.URL = c.Width, c.Height, c.URL
		it.Ext = extFromURL(c.URL)
	case 2: // wideo: największy wariant (video_versions zwyczajowo malejąco)
		c := igLargest(m.VideoVersions)
		if c == nil || c.URL == "" {
			return nil, fmt.Errorf("instagram: pozycja %d bez URL wideo", n)
		}
		it.IsImage = false
		it.Duration = int(m.VideoDuration)
		it.Width, it.Height, it.URL = c.Width, c.Height, c.URL
		it.Ext = "mp4"
	default:
		return nil, fmt.Errorf("instagram: nieobsługiwany typ elementu serii (%d)", m.MediaType)
	}
	return it, nil
}

// igCaption zwraca tekst opisu jako jedną linię (nowe linie → spacje).
func igCaption(m igMedia) string {
	if m.Caption == nil {
		return ""
	}
	text := m.Caption.Text
	text = strings.ReplaceAll(text, "\r", " ")
	text = strings.ReplaceAll(text, "\n", " ")
	return strings.TrimSpace(text)
}

// igLargest zwraca wariant o największym polu (fallback na [0], gdy brak wymiarów).
func igLargest(cands []igCandidate) *igCandidate {
	var best *igCandidate
	for i := range cands {
		c := &cands[i]
		if c.URL == "" {
			continue
		}
		if best == nil {
			best = c
			continue
		}
		if c.Width*c.Height > best.Width*best.Height {
			best = c
		}
	}
	return best
}

// extFromURL wyciąga rozszerzenie pliku ze ścieżki URL — whitelist:
// jpg/png/webp; cokolwiek innego (np. "avif") tremuje do "jpg" (pełny wariant
// IG daje JPEG — a dla wideo mp4 ustawia caller).
func extFromURL(raw string) string {
	path := raw
	if u, err := url.Parse(raw); err == nil {
		path = u.Path
	}
	dot := strings.LastIndex(path, ".")
	if dot < 0 {
		return "jpg"
	}
	switch strings.ToLower(path[dot+1:]) {
	case "jpg", "jpeg":
		return "jpg"
	case "png":
		return "png"
	case "webp":
		return "webp"
	default:
		return "jpg"
	}
}

// truncCaption skraca tekst do max rune (rune-safe: PL/EMOJI nie rozcina).
func truncCaption(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…"
}

// RunDirectDownloadWithProgress pobiera plik wprost HTTP (obrazy IG),
// zapisuje atomowo (.part → rename) i raportuje postęp 0-100.
// Limit czasu egzekwuje ctx (maxDownloadTime po stronie api).
func RunDirectDownloadWithProgress(ctx context.Context, rawURL, outputPath string, cb ProgressCallback) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return fmt.Errorf("zdjęcie: budowa żądania: %w", err)
	}
	req.Header.Set("User-Agent", browserUA)
	req.Header.Set("Referer", "https://www.instagram.com/")

	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return fmt.Errorf("zdjęcie: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("zdjęcie: HTTP %d od CDN", resp.StatusCode)
	}

	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return fmt.Errorf("zdjęcie: folder wyjściowy: %w", err)
	}
	tmp := outputPath + ".part"
	out, err := os.Create(tmp)
	if err != nil {
		return fmt.Errorf("zdjęcie: zapis: %w", err)
	}
	clean := true
	defer func() {
		if clean {
			out.Close()
			os.Remove(tmp)
		}
	}()

	total := resp.ContentLength
	var sent int64
	buf := make([]byte, 64*1024)
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := out.Write(buf[:n]); werr != nil {
				return fmt.Errorf("zdjęcie: zapis: %w", werr)
			}
			sent += int64(n)
			if total > 0 && cb != nil {
				cb(float64(sent)*100/float64(total), "", "")
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return fmt.Errorf("zdjęcie: odczyt: %w", rerr)
		}
	}
	if err := out.Close(); err != nil {
		return fmt.Errorf("zdjęcie: zapis: %w", err)
	}
	clean = false
	if err := os.Rename(tmp, outputPath); err != nil {
		return fmt.Errorf("zdjęcie: przenoszenie: %w", err)
	}
	if cb != nil {
		cb(100, "", "")
	}
	return nil
}

// jpegDimensions czyta nagłówek JPEG i zwraca wymiary z pierwszego SOF
// (0xC0-0xCF bez C4/C8/CC). Detekcja wymiarów daje klientowi realną
// rozdzielczość zdjęcia zamiast "0×0".
func jpegDimensions(r io.Reader) (int, int, error) {
	br := bufio.NewReader(io.LimitReader(r, 256*1024))
	var two [2]byte
	if _, err := io.ReadFull(br, two[:]); err != nil {
		return 0, 0, errors.New("jpeg: brak danych")
	}
	if two[0] != 0xFF || two[1] != 0xD8 {
		return 0, 0, errors.New("jpeg: brak SOI")
	}
	for {
		if _, err := io.ReadFull(br, two[:]); err != nil {
			return 0, 0, errors.New("jpeg: koniec nagłówka")
		}
		if two[0] != 0xFF {
			return 0, 0, errors.New("jpeg: uszkodzony marker")
		}
		for two[1] == 0xFF {
			if _, err := io.ReadFull(br, two[1:]); err != nil {
				return 0, 0, errors.New("jpeg: koniec nagłówka")
			}
		}
		marker := two[1]
		if marker == 0x01 || (marker >= 0xD0 && marker <= 0xD7) {
			continue // markery bez payloadu
		}
		var lenb [2]byte
		if _, err := io.ReadFull(br, lenb[:]); err != nil {
			return 0, 0, errors.New("jpeg: koniec nagłówka")
		}
		length := int(lenb[0])<<8 | int(lenb[1])
		if length < 2 {
			return 0, 0, errors.New("jpeg: zła długość segmentu")
		}
		payload := make([]byte, length-2)
		if _, err := io.ReadFull(br, payload); err != nil {
			return 0, 0, errors.New("jpeg: koniec nagłówka")
		}
		if marker >= 0xC0 && marker <= 0xCF && marker != 0xC4 && marker != 0xC8 && marker != 0xCC {
			if len(payload) < 5 {
				return 0, 0, errors.New("jpeg: krótki SOF")
			}
			h := int(payload[1])<<8 | int(payload[2])
			w := int(payload[3])<<8 | int(payload[4])
			return w, h, nil
		}
	}
}
