package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"social-downloader-engine/internal/ads"
	"social-downloader-engine/internal/downloader"
	lic "social-downloader-engine/internal/license"
)

// JobStatus reprezentuje stan zadania pobierania.
type JobStatus string

const (
	JobStarted   JobStatus = "started"
	JobRunning   JobStatus = "running"
	JobCompleted JobStatus = "completed"
	JobFailed    JobStatus = "failed"
)

const (
	// maxInfoTime — twardy limit pojedynczej ekstrakcji metadanych yt-dlp.
	maxInfoTime = 3 * time.Minute
	// maxDownloadTime — twardy limit pojedynczego pobierania (kill yt-dlp po tym czasie,
	// inaczej przerwane pobieranie zostawia processy zombie na zawsze).
	maxDownloadTime = 30 * time.Minute
	// jobRetention — jak długo zadania (w tym zakończone) trzymane są w pamięci,
	// zanim janitor je usunie (mapa jobs rosłaby bez ogrania).
	jobRetention = 2 * time.Hour
	// maxHeightFree — maks. wysokość wideo dostępna bez premium.
	maxHeightFree = 720
)

// Job to pojedyncze zadanie pobierania.
type Job struct {
	ID         string    `json:"job_id"`
	Status     JobStatus `json:"status"`
	URL        string    `json:"url"`
	FormatID   string    `json:"format_id"`
	OutputPath string    `json:"output_path"`
	Progress   float64   `json:"progress"`
	Error      string    `json:"error,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

// Config konfiguruje serwer API.
type Config struct {
	Addr          string // adres nasłuchu (np. 127.0.0.1:8765)
	YtDlpPath     string
	FfmpegPath    string
	License       *lic.License
	MaxYouTubeSec int
	Ads           *ads.Config
	// IgCookie — cookie sesji Instagram (lub "") z pliku IgCookieFile
	// (UTF-8 bez BOM obok executable). Zarządzane pod igMu.
	IgCookie     string
	IgCookieFile string
}

// maxIgCookieLen — twardy limit POST /ig-cookie (nagłówek Cookie ma limit ~8 KB;
// zapas zmieści pełny, rozwinięty ciąg), chroni też plik na dysku.
const maxIgCookieLen = 16 << 10

// Server zawiera zależności handlerów oraz referencję do http.Server.
type Server struct {
	cfg  *Config
	srv  *http.Server
	jobs map[string]*Job
	mu   sync.Mutex
	igMu sync.Mutex // chroni cfg.IgCookie oraz plik IgCookieFile
}

func New(cfg *Config) *Server {
	s := &Server{
		cfg:  cfg,
		jobs: make(map[string]*Job),
	}
	// http.Server konstruowany w New, po czym pole srv NIGDY nie jest
	// nadpisywane — Start (goroutine) i Shutdown (główny wątek) nie ścigają się.
	s.srv = &http.Server{
		Addr:    cfg.Addr,
		Handler: s.Router(),
	}
	// janitor: cykliczne usuwanie starych zadań
	go s.pruneJobsLoop()
	return s
}

// Router zwraca http.Handler z wszystkimi endpointami.
func (s *Server) Router() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/info", s.handleInfo)
	mux.HandleFunc("/download", s.handleDownload)
	mux.HandleFunc("/progress", s.handleProgress)
	mux.HandleFunc("/file", s.handleFile)
	mux.HandleFunc("/healthz", s.handleHealthz)
	mux.HandleFunc("/license", s.handleLicense)
	mux.HandleFunc("/ads", s.handleAds)
	mux.HandleFunc("/ig-cookie", s.handleIgCookie)
	return mux
}

// Start uruchamia serwer HTTP (blokujące).
func (s *Server) Start() error {
	if err := s.srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

// Shutdown zamyka serwer HTTP graceful.
func (s *Server) Shutdown(ctx context.Context) error {
	if s.srv == nil {
		return nil
	}
	return s.srv.Shutdown(ctx)
}

// pruneJobsLoop cyklicznie usuwa zadania starsze niż jobRetention.
func (s *Server) pruneJobsLoop() {
	t := time.NewTicker(30 * time.Minute)
	defer t.Stop()
	for range t.C {
		cutoff := time.Now().Add(-jobRetention)
		s.mu.Lock()
		for id, j := range s.jobs {
			if j.CreatedAt.Before(cutoff) {
				delete(s.jobs, id)
			}
		}
		s.mu.Unlock()
	}
}

// addJob rejestruje nowe zadanie.
func (s *Server) addJob(id, url, formatID, outputPath string) *Job {
	s.mu.Lock()
	defer s.mu.Unlock()
	j := &Job{
		ID:         id,
		Status:     JobStarted,
		URL:        url,
		FormatID:   formatID,
		OutputPath: outputPath,
		Progress:   0,
		CreatedAt:  time.Now(),
	}
	s.jobs[id] = j
	return j
}

// updateJob aktualizuje status i postęp zadania.
func (s *Server) updateJob(id string, status JobStatus, progress float64, err string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if j, ok := s.jobs[id]; ok {
		j.Status = status
		j.Progress = progress
		if err != "" {
			j.Error = err
		}
	}
}

// getJob zwraca KOPIĘ zadania po id. Kopiowanie pod mutexem jest istotne:
// zwrócenie wskaźnika i czytanie pól w handleProgress to data race
// z goroutinem pobierania, który te pola updateuje pod mutexem.
func (s *Server) getJob(id string) (Job, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	j, ok := s.jobs[id]
	if !ok {
		return Job{}, false
	}
	return *j, true
}

// ----- endpointy -----

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"ok"}`))
}

// ----- pozycje publikacji (kontrakt v2) -----

// Entry — jedna pozycja publikacji ujednolicona dla klienta: pojedyncze wideo
// LUB element serii/zdjęcia. Któś krytyczny zbierało się po staremu; pola
// "entries"/"is_playlist"/"playlist_note" są DODATKOWE do starych kluczy,
// więc starsze klienci działają bez zmian.
type Entry struct {
	Index     int                 `json:"index"`
	ID        string              `json:"id"`
	Title     string              `json:"title"`
	Duration  int                 `json:"duration"`
	IsImage   bool                `json:"is_image"`
	Qualities []int               `json:"qualities"`
	Formats   []downloader.Format `json:"formats"`
}

// isImageInfo rozpoznaje syntetyczne metadane zdjęcia (jeden format, VCodec "image").
func isImageInfo(vi *downloader.VideoInfo) bool {
	return len(vi.Formats) == 1 && vi.Formats[0].VCodec == "image"
}

func entriesFromInfo(vi *downloader.VideoInfo) []Entry {
	return []Entry{{
		Index:     1,
		ID:        vi.ID,
		Title:     vi.Title,
		Duration:  vi.Duration,
		IsImage:   isImageInfo(vi),
		Qualities: downloader.AvailableQualities(vi.Formats),
		Formats:   vi.Formats,
	}}
}

// resolveIGPhotoFallback: gdy yt-dlp zgłosi ErrNoVideoFormats i URL jest
// postem IG, rozwiązujemy zdjęcie anonimowo (resolver). Zwraca (vi, fileURL,
// isPhoto, err); fileURL != "" ⇒ pobieranie przez RunDirectDownloadWithProgress.
func resolveIGPhotoFallback(ctx context.Context, err error, rawURL string) (*downloader.VideoInfo, string, bool, error) {
	if !errors.Is(err, downloader.ErrNoVideoFormats) {
		return nil, "", false, err
	}
	code := downloader.InstagramShortcode(rawURL)
	if code == "" {
		return nil, "", false, err
	}
	pvi, fileURL, rerr := downloader.ResolveInstagramPhoto(ctx, code)
	if rerr != nil {
		return nil, "", false, rerr
	}
	return pvi, fileURL, true, nil
}

// isSessionBroken rozpoznaje błąd resolvera sesyjnego jako „sesja wygasła/
// nieprawidłowa" — via sentinel albo po tekście (zweryfikowana podwójność,
// bo client.Do może opakować błąd z CheckRedirect w *url.Error).
func isSessionBroken(err error) bool {
	if errors.Is(err, downloader.ErrSessionInvalid) {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "wygasła") || strings.Contains(msg, "nieprawidłowa")
}

// igCookieSnapshot zwraca aktualne cookie sesji IG ("", gdy nieustawione).
func (s *Server) igCookieSnapshot() string {
	s.igMu.Lock()
	defer s.igMu.Unlock()
	return s.cfg.IgCookie
}

// setIgCookie zapisuje sesję IG: do pliku (UTF-8 bez BOM) i w pamięci.
// Pusty string kasuje plik i czyści w pamięci. Wywołanie pod igMu.
func (s *Server) setIgCookie(cookie string) error {
	s.igMu.Lock()
	defer s.igMu.Unlock()
	if s.cfg.IgCookieFile == "" {
		return errors.New("silnik nie skonfigurował pliku sesji IG")
	}
	cookie = strings.TrimSpace(cookie)
	if cookie == "" {
		s.cfg.IgCookie = ""
		if err := os.Remove(s.cfg.IgCookieFile); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	if len(cookie) > maxIgCookieLen {
		return fmt.Errorf("cookie zbyt długie (max %d znaków)", maxIgCookieLen)
	}
	if err := os.WriteFile(s.cfg.IgCookieFile, []byte(cookie), 0o600); err != nil {
		return err
	}
	s.cfg.IgCookie = cookie
	return nil
}

// handleIgCookie: GET → {"configured":bool} (wartość cookie NIE jest zwracana);
// POST {"cookie":"..."} → zapis + {"ok":true,"configured":bool}. Pusty cookie
// kasuje sesję ({"ok":true,"configured":false}).
func (s *Server) handleIgCookie(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, map[string]interface{}{"configured": s.igCookieSnapshot() != ""})
	case http.MethodPost:
		body, err := io.ReadAll(io.LimitReader(r.Body, maxIgCookieLen+1024))
		if err != nil {
			http.Error(w, "błąd odczytu body", http.StatusBadRequest)
			return
		}
		defer r.Body.Close()
		var req struct {
			Cookie string `json:"cookie"`
		}
		if err := json.Unmarshal(body, &req); err != nil {
			http.Error(w, "nieprawidłowy JSON", http.StatusBadRequest)
			return
		}
		if err := s.setIgCookie(req.Cookie); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]interface{}{
			"ok":         true,
			"configured": strings.TrimSpace(req.Cookie) != "",
		})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// licenseInfo zwraca (features, hwid) dla odpowiedzi /info.
func (s *Server) licenseInfo() ([]string, string) {
	if s.cfg.License != nil {
		return s.cfg.License.Features, s.cfg.License.HardwareID
	}
	return []string{}, ""
}

// entriesFromPostItems zamienia elementy sesyjnego resolvera IG na Entry
// kontraktu v2: syntetyczne formaty "img" (obraz) / "vid" (wideo) per element.
func entriesFromPostItems(items []downloader.PostItem) []Entry {
	out := make([]Entry, 0, len(items))
	for _, it := range items {
		var formats []downloader.Format
		if it.IsImage {
			formats = []downloader.Format{{
				FormatID:   "img",
				Ext:        it.Ext,
				Width:      it.Width,
				Height:     it.Height,
				VCodec:     "image",
				ACodec:     "none",
				FormatNote: "obraz (pełna jakość)",
			}}
		} else {
			formats = []downloader.Format{{
				FormatID:   "vid",
				Ext:        "mp4",
				Width:      it.Width,
				Height:     it.Height,
				VCodec:     "h264",
				ACodec:     "aac",
				FormatNote: "wideo (pełna jakość)",
			}}
		}
		quals := []int{}
		if it.Height > 0 {
			quals = []int{it.Height}
		}
		out = append(out, Entry{
			Index:     it.Index,
			ID:        it.ID,
			Title:     it.Title,
			Duration:  it.Duration,
			IsImage:   it.IsImage,
			Qualities: quals,
			Formats:   formats,
		})
	}
	return out
}

// postSessionNote — nota /info przy aktywnym sesyjnym resolverze (również dla
// pojedynczego elementu — żeby user wiedział, że sesja działa).
const postSessionNote = "Sesja IG aktywna — wszystkie pozycje dostępne."

// noteSessionStale — nota, gdy cookie ustawione, ale sesyjny resolver odrzucił.
const noteSessionStale = "Sesja IG ustawiona, ale wygasła/nieaktywna — odśwież cookie w ustawieniach."

// tryHandleIGPostList: przy ErrNoVideoFormats na poście IG próbuje sesyjnego
// resolvera (cookie z /ig-cookie). Zwraca pozycje i top-level VideoInfo z
// WIDOCZNEGO pierwszego elementu; error = brak sesji/awaria resolvera → caller
// prowadzi ścieżkę anonimową.
func (s *Server) tryResolveIGPost(ctx context.Context, err error, rawURL string) (*downloader.VideoInfo, []downloader.PostItem, error) {
	if !errors.Is(err, downloader.ErrNoVideoFormats) {
		return nil, nil, err
	}
	code := downloader.InstagramShortcode(rawURL)
	if code == "" {
		return nil, nil, err
	}
	cookie := s.igCookieSnapshot()
	if cookie == "" {
		return nil, nil, err // anonimowo — brak sesji
	}
	svi, items, rerr := downloader.ResolveInstagramPost(ctx, code, cookie)
	if rerr != nil {
		return nil, nil, rerr
	}
	return svi, items, nil
}

func (s *Server) handleInfo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	rawURL := r.URL.Query().Get("url")
	if err := validateURL(rawURL); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), maxInfoTime)
	defer cancel()
	vi, err := downloader.RunInfo(ctx, s.cfg.YtDlpPath, rawURL)
	isPhoto := false
	sessionBad := ""
	if err != nil {
		if errors.Is(err, downloader.ErrPrivate) {
			http.Error(w, "treść jest prywatna lub niedostępna", http.StatusForbidden)
			return
		}
		// Sesyjna ścieżka IG: pełna lista elementów serii (jeśli sesja jest i działa).
		if svi, items, serr := s.tryResolveIGPost(ctx, err, rawURL); serr == nil {
			features, hwid := s.licenseInfo()
			entries := entriesFromPostItems(items)
			resp := map[string]interface{}{
				"id":            svi.ID,
				"title":         svi.Title,
				"duration":      svi.Duration,
				"uploader":      svi.Uploader,
				"qualities":     entries[0].Qualities,
				"formats":       entries[0].Formats,
				"premium":       s.cfg.License != nil,
				"features":      features,
				"hwid":          hwid,
				"is_playlist":   true,
				"entries":       entries,
				"playlist_note": postSessionNote,
			}
			writeJSON(w, resp)
			return
		} else if isSessionBroken(serr) {
			sessionBad = serr.Error()
		} else {
			log.Printf("[info] sesyjny resolver IG nieudany (%v) — ścieżka anonimowa", serr)
		}

		var pvi *downloader.VideoInfo
		var isP bool
		pvi, _, isP, err = resolveIGPhotoFallback(ctx, err, rawURL)
		if err != nil {
			msg := err.Error()
			if sessionBad != "" {
				msg = noteSessionStale + " " + msg
			}
			http.Error(w, msg, http.StatusBadGateway)
			return
		}
		vi, isPhoto = pvi, isP
	}

	if isYouTube(rawURL) && vi.Duration > s.cfg.MaxYouTubeSec {
		http.Error(w, fmt.Sprintf("YouTube: film dłuższy niż %d minut (limit)", s.cfg.MaxYouTubeSec/60), http.StatusForbidden)
		return
	}

	features, hwid := s.licenseInfo()

	note := ""
	if isPhoto {
		if sessionBad != "" {
			note = noteSessionStale + " Anonimowo dostępny jest pierwszy element serii."
		} else {
			note = "Instagram: pełna lista zdjęć z serii wymaga sesji IG (ustaw w kliencie) — anonimowo dostępny jest pierwszy element."
		}
	}
	resp := map[string]interface{}{
		"id":            vi.ID,
		"title":         vi.Title,
		"duration":      vi.Duration,
		"uploader":      vi.Uploader,
		"qualities":     downloader.AvailableQualities(vi.Formats),
		"formats":       vi.Formats,
		"premium":       s.cfg.License != nil,
		"features":      features,
		"hwid":          hwid,
		"is_playlist":   isPhoto,
		"entries":       entriesFromInfo(vi),
		"playlist_note": note,
	}
	writeJSON(w, resp)
}

func (s *Server) handleDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "błąd odczytu body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var req struct {
		URL        string `json:"url"`
		FormatID   string `json:"format_id"`
		OutputPath string `json:"output_path"`
		Item       int    `json:"item"` // 1-based element serii (0/brak = 1)
	}
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "nieprawidłowy JSON", http.StatusBadRequest)
		return
	}
	if req.Item == 0 {
		req.Item = 1 // domyślnie pierwszy element
	}
	if req.URL == "" || req.FormatID == "" || req.OutputPath == "" {
		http.Error(w, "brak wymaganych pól: url, format_id, output_path", http.StatusBadRequest)
		return
	}
	if err := validateURL(req.URL); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := validateFormatID(req.FormatID); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if !isAllowedOutputPath(req.OutputPath) {
		http.Error(w, "ścieżka wyjściowa musi być w katalogu domowym użytkownika", http.StatusBadRequest)
		return
	}

	// Walidacja treści fail-closed: bez metadanych NIE MA pobierania.
	// Jedna ekstrakcja obsługuje naraz: prywatność, limit 10 min YouTube
	// (klienci pomijający /info nie mogą tym obejść) i bramkę HD premium.
	// Dla postów IG (brak wideo w yt-dlp): najpierw sesyjny resolver (pełna
	// seria), fallback anonimowy (pierwszy element); pobieranie strumieniowe.
	ctx, cancel := context.WithTimeout(r.Context(), maxInfoTime)
	defer cancel()
	vi, err := downloader.RunInfo(ctx, s.cfg.YtDlpPath, req.URL)
	directURL := ""
	sessionBad := false
	if err != nil {
		if errors.Is(err, downloader.ErrPrivate) {
			http.Error(w, "treść jest prywatna lub niedostępna", http.StatusForbidden)
			return
		}
		// Ścieżka sesyjna: pełna lista elementów; /download przyjmuje "item".
		if svi, items, serr := s.tryResolveIGPost(ctx, err, req.URL); serr == nil {
			if req.Item < 1 || req.Item > len(items) {
				http.Error(w, fmt.Sprintf("parametr item poza zakresem: dopuszczalne 1..%d", len(items)), http.StatusBadRequest)
				return
			}
			it := items[req.Item-1]
			want := "img"
			if !it.IsImage {
				want = "vid"
			}
			if req.FormatID != want {
				http.Error(w, fmt.Sprintf("format_id %q nie pasuje do pozycji %d (dla %s oczekiwano %q)",
					req.FormatID, req.Item, map[bool]string{true: "obrazu", false: "wideo"}[it.IsImage], want), http.StatusBadRequest)
				return
			}
			vi = svi
			directURL = it.URL
		} else if isSessionBroken(serr) {
			sessionBad = true
		} else {
			log.Printf("[download] sesyjny resolver IG nieudany (%v) — ścieżka anonimowa", serr)
		}
		// Ścieżka anonimowa (także, gdy sesja wygasła): pierwszy element postu.
		if directURL == "" {
			var pvi *downloader.VideoInfo
			var fu string
			pvi, fu, _, err = resolveIGPhotoFallback(ctx, err, req.URL)
			if err != nil {
				msg := err.Error()
				if sessionBad {
					msg = noteSessionStale + " " + msg
				}
				http.Error(w, msg, http.StatusBadGateway)
				return
			}
			if req.Item != 1 {
				http.Error(w, "pełna lista serii wymaga sesji IG", http.StatusBadRequest)
				return
			}
			vi, directURL = pvi, fu
		}
	}
	if isYouTube(req.URL) && vi.Duration > s.cfg.MaxYouTubeSec {
		http.Error(w, fmt.Sprintf("YouTube: film dłuższy niż %d minut (limit)", s.cfg.MaxYouTubeSec/60), http.StatusForbidden)
		return
	}

	// Premium: rozdzielczości powyżej 720p wymagają feature "hd" (fail-closed:
	// brak licencji lub brak cechy = blokada). Zasada obejmuje też formaty
	// łączone ("135+140") i selektory najlepszej jakości ("bestvideo"), które
	// nie dają się zweryfikować przeciwko wysokości — dla darmowych blokujemy.
	// Bramką HD objęta jest TYLKO ścieżka yt-dlp (directURL == "") — elementy
	// IG pobierane wprost (obrazy anonimowe i sesyjne "img"/"vid" nie mają
	// formatów yt-dlp, bramka juebe biłaby sesyjne wideo >720p darmowych).
	if directURL == "" && !isImageInfo(vi) && (s.cfg.License == nil || !s.cfg.License.HasFeature("hd")) {
		if premiumHDBlocked(vi, req.FormatID) {
			http.Error(w, "rozdzielczość powyżej 720p wymaga wersji premium", http.StatusForbidden)
			return
		}
	}

	if err := os.MkdirAll(filepath.Dir(req.OutputPath), 0755); err != nil {
		http.Error(w, "nie można utworzyć folderu wyjściowego", http.StatusInternalServerError)
		return
	}

	jobID := fmt.Sprintf("job-%d", time.Now().UnixNano())
	s.addJob(jobID, req.URL, req.FormatID, req.OutputPath)

	// Kontekst pobierania niezależny od żądania HTTP (klient potem odpytuje
	// /progress), ale z twardym limitem czasu.
	go func() {
		s.updateJob(jobID, JobRunning, 0, "")
		dlCtx, dlCancel := context.WithTimeout(context.Background(), maxDownloadTime)
		defer dlCancel()
		var err error
		if directURL != "" {
			// Element IG (sesyjny lub anonimowy): bezpośrednie pobranie z CDN.
			err = downloader.RunDirectDownloadWithProgress(
				dlCtx, directURL, req.OutputPath,
				func(percent float64, speed, eta string) {
					s.updateJob(jobID, JobRunning, percent, "")
				},
			)
		} else {
			err = downloader.RunDownloadWithProgress(
				dlCtx, s.cfg.YtDlpPath, s.cfg.FfmpegPath,
				req.URL, req.FormatID, req.OutputPath,
				func(percent float64, speed, eta string) {
					s.updateJob(jobID, JobRunning, percent, "")
				},
			)
		}
		if err != nil {
			s.updateJob(jobID, JobFailed, 0, err.Error())
			log.Printf("[download %s] błąd: %v", jobID, err)
		} else {
			s.updateJob(jobID, JobCompleted, 100, "")
			log.Printf("[download %s] zakończone: %s", jobID, req.OutputPath)
		}
	}()

	writeJSON(w, map[string]interface{}{"job_id": jobID, "status": JobStarted})
}

func (s *Server) handleProgress(w http.ResponseWriter, r *http.Request) {
	jobID := r.URL.Query().Get("job_id")
	if jobID == "" {
		http.Error(w, "brak parametru job_id", http.StatusBadRequest)
		return
	}

	job, ok := s.getJob(jobID)
	if !ok {
		http.Error(w, "nie znaleziono zadania", http.StatusNotFound)
		return
	}

	writeJSON(w, map[string]interface{}{
		"job_id":      job.ID,
		"status":      job.Status,
		"progress":    job.Progress,
		"error":       job.Error,
		"output_path": job.OutputPath,
		"url":         job.URL,
	})
}

// contentTypes — deterministyczne MIME dla rozszerzeń, które realnie pojawiają
// się w pobieraniach; mime.TypeByExtension na Windows potrafi zwracać wpisy
// z rejestru (np. "application/mp4" albo puste), więc te typowe nadpisujemy.
var contentTypes = map[string]string{
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".png":  "image/png",
	".webp": "image/webp",
	".gif":  "image/gif",
	".bmp":  "image/bmp",
	".mp4":  "video/mp4",
	".webm": "video/webm",
	".mkv":  "video/x-matroska",
	".mov":  "video/quicktime",
	".m4v":  "video/x-m4v",
	".m4a":  "audio/mp4",
	".mp3":  "audio/mpeg",
	".ogg":  "audio/ogg",
	".wav":  "audio/wav",
}

// contentTypeFor — MIME po rozszerzeniu z bezpiecznym octet-stream fallbackiem.
func contentTypeFor(name string) string {
	if ct, ok := contentTypes[strings.ToLower(filepath.Ext(name))]; ok {
		return ct
	}
	if ct := mime.TypeByExtension(strings.ToLower(filepath.Ext(name))); ct != "" {
		return ct
	}
	return "application/octet-stream"
}

// handleFile: GET /file?job_id=<id> — przesłanie binaryjne gotowego pliku
// zakończonego zadania. Fail-closed jak /download: plik serializuje się
// wyłącznie, gdy (1) zadanie jest completed, (2) OutputPath nadal jest w
// katalogu domowym (walidacja w momencie pobrania, nie tylko przy starcie joba)
// i (3) plik istnieje na dysku. Klienci optimistic odpytują /progress i wywołują
// /file dopiero po completed — endpoint i tak nie wypuści niegotowego pliku.
func (s *Server) handleFile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	jobID := r.URL.Query().Get("job_id")
	if jobID == "" {
		http.Error(w, "brak parametru job_id", http.StatusBadRequest)
		return
	}
	job, ok := s.getJob(jobID)
	if !ok {
		http.Error(w, "nie znaleziono zadania", http.StatusNotFound)
		return
	}
	if job.Status != JobCompleted {
		msg := fmt.Sprintf("plik dostępny po zakończeniu pobierania (status: %s)", job.Status)
		if job.Status == JobFailed && job.Error != "" {
			msg = fmt.Sprintf("pobieranie nieudane: %s", job.Error)
		}
		http.Error(w, msg, http.StatusConflict)
		return
	}
	// Fail-closed: ta sama polityka, co /download — tylko katalog domowy.
	// Sprawdzane ponownie, bo OutputPath w jobie mógłby w teorii pochodzić
	// z innej konfiguracji silnika (oszukanie kontroli przy starcie).
	if !isAllowedOutputPath(job.OutputPath) {
		http.Error(w, "ścieżka wyjściowa musi być w katalogu domowym użytkownika", http.StatusForbidden)
		return
	}
	fi, err := os.Stat(job.OutputPath)
	if err != nil {
		http.Error(w, "plik nie znaleziono na dysku", http.StatusNotFound)
		return
	}
	if fi.IsDir() {
		http.Error(w, "ścieżka wyjściowa nie jest plikiem", http.StatusNotFound)
		return
	}
	f, err := os.Open(job.OutputPath)
	if err != nil {
		http.Error(w, "nie można otworzyć pliku", http.StatusInternalServerError)
		return
	}
	defer f.Close()

	name := filepath.Base(job.OutputPath)
	w.Header().Set("Content-Type", contentTypeFor(name))
	// mime.FormatMediaType dodaje RFC 5987 (filename=UTF-8''), gdy nazwa nie
	// jest czystym ASCII — polskie tytuły publikacji wchodzą przez ten gałąz.
	if cd := mime.FormatMediaType("attachment", map[string]string{"filename": name}); cd != "" {
		w.Header().Set("Content-Disposition", cd)
	}
	http.ServeContent(w, r, "", fi.ModTime(), f)
}

func (s *Server) handleLicense(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.cfg.License == nil {
		writeJSON(w, map[string]interface{}{"premium": false, "features": []string{}, "hwid": ""})
		return
	}
	writeJSON(w, map[string]interface{}{
		"premium":  true,
		"hwid":     s.cfg.License.HardwareID,
		"features": s.cfg.License.Features,
	})
}

func (s *Server) handleAds(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	typ := r.URL.Query().Get("type")
	if typ == "" {
		typ = "banner"
	}

	var ad interface{}
	// Premium: bez reklam (spec: jednorazowe odblokowanie usuwa banery i interstitial).
	if s.cfg.License == nil && s.cfg.Ads != nil {
		ad = s.cfg.Ads.Pick(typ)
	}

	writeJSON(w, map[string]interface{}{
		"type":    typ,
		"ad":      ad,
		"premium": s.cfg.License != nil,
	})
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func isYouTube(url string) bool {
	return strings.Contains(url, "youtube.com") || strings.Contains(url, "youtu.be")
}

// ----- walidacja wejść -----

// yt-dlp dostaje url i format_id jako argumenty; argument zaczynający się od "-"
// byłby zinterpretowany jako flaga (np. --exec → wykonanie dowolnego kodu,
// --write-pages → zapis treści gdziekolwiek). Whitelisteliminują te iniekcje.

// validateURL wymaga protokołu http(s):// — wyklucza argumenty-flagi yt-dlp.
func validateURL(raw string) error {
	u := strings.TrimSpace(raw)
	if u == "" {
		return errors.New("brak parametru url")
	}
	if len(u) > 2048 {
		return errors.New("url zbyt długi")
	}
	low := strings.ToLower(u)
	if !strings.HasPrefix(low, "http://") && !strings.HasPrefix(low, "https://") {
		return errors.New("url musi zaczynać się od http:// lub https://")
	}
	return nil
}

// formatIDRe — whitelist znaków format_id: lettera/cyfra na start (bez flagi
// "-" i bez znacznika "." lub "+"), reszta: litery, cyfry, _ . + -, łącznie
// z łączonymi ID ("137+140"). Blokuje iniekcje typu "--exec ...".
var formatIDRe = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.+-]{0,63}$`)

func validateFormatID(f string) error {
	if !formatIDRe.MatchString(f) {
		return errors.New("nieprawidłowy format_id (dozwolone: litery, cyfry, _ . + -)")
	}
	return nil
}

// isAllowedOutputPath dopuszcza TYLKO ścieżki w katalogu domowym użytkownika —
// inaczej żądanie mogłoby nadpisać dowolny plik w systemie (np. w C:\Windows).
func isAllowedOutputPath(p string) bool {
	home, err := os.UserHomeDir()
	if err != nil {
		return false
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return false
	}
	home = filepath.Clean(home)
	if strings.EqualFold(abs, home) {
		return true
	}
	// Windows: ścieżki nirozróżniające wielkości liter — porównanie bezcase.
	sep := string(os.PathSeparator)
	return strings.HasPrefix(strings.ToLower(abs), strings.ToLower(home)+sep)
}

// premiumHDBlocked sprawdza czy format_id (również łączony "a+b") przekracza
// 720p albo zawiera część-selektor ("best...", "b"), której wysokości nie
// można potwierdzić na liście formatów.
func premiumHDBlocked(vi *downloader.VideoInfo, formatID string) bool {
	for _, part := range strings.Split(formatID, "+") {
		if part == "" {
			return true
		}
		resolved := false
		for _, f := range vi.Formats {
			if f.FormatID == part {
				if f.Height > maxHeightFree {
					return true
				}
				resolved = true
				break
			}
		}
		if !resolved {
			low := strings.ToLower(part)
			if strings.HasPrefix(low, "best") || low == "b" {
				return true
			}
		}
	}
	return false
}
