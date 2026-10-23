package downloader

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"strings"
)

// ErrPrivate oznacza treść prywatną/niedostępną — rozpoznawaną po stderr yt-dlp.
var ErrPrivate = errors.New("treść jest prywatna lub niedostępna")

// VideoInfo to przetworzone metadane zwracane przez yt-dlp (-J).
type VideoInfo struct {
	ID       string   `json:"id"`
	Title    string   `json:"title"`
	Duration int      `json:"duration"` // sekundy
	Uploader string   `json:"uploader"`
	Formats  []Format `json:"formats"`
}

// Format reprezentuje jeden dostępny format wideo/dźwięku.
type Format struct {
	FormatID   string  `json:"format_id"`
	Ext        string  `json:"ext"`
	Height     int     `json:"height"`
	Width      int     `json:"width"`
	Filesize   int64   `json:"filesize"`
	FPS        float64 `json:"fps"`
	VCodec     string  `json:"vcodec"`
	ACodec     string  `json:"acodec"`
	FormatNote string  `json:"format_note"`
}

// ProgressCallback jest wywoływany z postępem pobierania (0-100).
type ProgressCallback func(percent float64, speed string, eta string)

// RunInfo wywołuje yt-dlp -J <url> i parsuje JSON.
// Treści prywatne/niedostępne wykrywane są po stderr yt-dlp (wiarygodne źródło),
// a nie po heurystyce w tytule (fałszywe trafienia, np. tytuł "Prywatne kursy wideo").
func RunInfo(ctx context.Context, ytDlpPath, url string) (*VideoInfo, error) {
	cmd := exec.CommandContext(ctx, ytDlpPath, "-J", "--no-playlist", "--no-warnings", url)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		errText := strings.ToLower(stderr.String())
		switch {
		case strings.Contains(errText, "private"),
			strings.Contains(errText, "unavailable"),
			strings.Contains(errText, "sign in to confirm"),
			strings.Contains(errText, "login required"):
			return nil, ErrPrivate
		case strings.Contains(errText, "no video formats found"):
			// Post bez wideo (np. IG ze zdjęciem/serią) — api ma fallback na
			// resolver zdjęć, chyba że URL nie daje się rozpoznać jako IG.
			return nil, ErrNoVideoFormats
		}
		return nil, fmt.Errorf("yt-dlp nie mógł pobrać metadanych: %w", err)
	}

	var vi VideoInfo
	if err := json.Unmarshal(stdout.Bytes(), &vi); err != nil {
		return nil, fmt.Errorf("nie można sparsować wyjścia yt-dlp: %w", err)
	}

	return &vi, nil
}

// RunDownloadWithProgress uruchamia yt-dlp i wywołuje cb z postępem (0-100).
// ctx przerywa (kill) yt-dlp po upływie limitu czasu albo anulowaniu;
// brak tego ograniczenia pozostawia procesy zombie na zawsze wisiące.
func RunDownloadWithProgress(ctx context.Context, ytDlpPath, ffmpegPath, url, formatID, outputPath string, cb ProgressCallback) error {
	args := []string{
		"-f", formatID,
		"--no-playlist",
		"--no-warnings",
		"--newline",           // wymusza liniowy output (łatwiejsze parsowanie)
		"--progress-template", // szablon postępu (yt-dlp obsługuje)
		"%(progress.downloaded_bytes)s/%(progress.total_bytes)s %(progress.percent)s %(progress.speed)s %(progress.eta)s",
		"--ffmpeg-location", ffmpegPath,
		"-o", outputPath,
		url,
	}
	cmd := exec.CommandContext(ctx, ytDlpPath, args...)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("nie można otworzyć stdout: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("nie można otworzyć stderr: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("nie można uruchomić yt-dlp: %w", err)
	}

	// yt-dlp wysyła postęp na stderr (lub stdout z --newline). Czyścimy oba strumienie.
	go parseProgress(stdout, cb)
	go parseProgress(stderr, cb)

	if err := cmd.Wait(); err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("pobieranie przerwane (limit czasu): %w", ctx.Err())
		}
		return fmt.Errorf("pobieranie nie powiodło się: %w", err)
	}
	return nil
}

// progressRe dopasowuje linie typu: 123456/789012 45.2% 1.23MiB/s 0:12
var progressRe = regexp.MustCompile(`([\d.]+)%`)

func parseProgress(out io.Reader, cb ProgressCallback) {
	if cb == nil || out == nil {
		return
	}
	scanner := bufio.NewScanner(out)
	for scanner.Scan() {
		line := scanner.Text()
		m := progressRe.FindStringSubmatch(line)
		if len(m) >= 2 {
			var p float64
			fmt.Sscanf(m[1], "%f", &p)
			cb(p, "", "")
		}
	}
}

// AvailableQualities zwraca posortowaną listę unikalnych rozdzielczości wideo.
func AvailableQualities(formats []Format) []int {
	seen := make(map[int]bool)
	var res []int
	for _, f := range formats {
		if f.Height > 0 && f.VCodec != "none" && !seen[f.Height] {
			seen[f.Height] = true
			res = append(res, f.Height)
		}
	}
	for i := 0; i < len(res); i++ {
		for j := i + 1; j < len(res); j++ {
			if res[i] > res[j] {
				res[i], res[j] = res[j], res[i]
			}
		}
	}
	return res
}
