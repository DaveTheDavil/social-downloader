package updater

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const githubAPI = "https://api.github.com/repos/yt-dlp/yt-dlp/releases/latest"

// release asset z GitHub.
type releaseAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

// EnsureYtDlp sprawdza wersję yt-dlp i pobiera nowszą z GitHub releases, jeśli dostępna.
func EnsureYtDlp(ctx context.Context) error {
	installed, err := installedVersion()
	if err != nil {
		// jeśli brak yt-dlp, wymuś pobranie
		log.Printf("[updater] yt-dlp nie znaleziony (%v), wymuszam pobranie", err)
		installed = "0.0.0"
	}

	latest, err := latestVersion(ctx)
	if err != nil {
		log.Printf("[updater] błąd sprawdzenia wersji na GitHub: %v", err)
		return err
	}
	log.Printf("[updater] zainstalowana wersja=%q, najnowsza na GitHub=%q", installed, latest)

	if installed == latest {
		return nil // aktualny
	}

	log.Printf("[updater] pobieram yt-dlp %s...", latest)
	return downloadYtDlp(ctx, latest)
}

// BinaryPath zwraca nazwę pliku yt-dlp dla bieżącej platformy (z .exe na Windows).
// Ta sama nazwa jest jednocześnie assetem na GitHubie i plikiem obok aplikacji.
func BinaryPath() string {
	switch runtime.GOOS {
	case "windows":
		return "yt-dlp.exe"
	case "darwin":
		return "yt-dlp_macos"
	default:
		return "yt-dlp"
	}
}

// BinaryFilePath zwraca pełną ścieżkę yt-dlp obok executable aplikacji.
// Niezależna od katalogu roboczego — a exec.Command() celowo NIE szuka
// programów w CWD (Go 1.19+: "cannot run executable found relative to
// current directory"), więc bez pełnej ścieżki yt-dlp nigdy nie działa.
func BinaryFilePath() string {
	exe, err := os.Executable()
	if err != nil {
		return BinaryPath() // fallback: nazwa względna
	}
	return filepath.Join(filepath.Dir(exe), BinaryPath())
}

// installedVersion zwraca wersję yt-dlp (np. 2024.10.01).
func installedVersion() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, BinaryFilePath(), "--version")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// latestVersion pobiera tag najnowszego release z GitHub.
func latestVersion(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", githubAPI, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "social-downloader-engine")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var v struct {
		TagName string `json:"tag_name"`
	}
	if err := json.Unmarshal(body, &v); err != nil {
		return "", err
	}
	return strings.TrimPrefix(v.TagName, "v"), nil
}

// downloadYtDlp pobiera binary yt-dlp dla bieżącego OS/ARCH z GitHub releases.
func downloadYtDlp(ctx context.Context, version string) error {
	req, err := http.NewRequestWithContext(ctx, "GET", githubAPI, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "social-downloader-engine")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var rel struct {
		Assets []releaseAsset `json:"assets"`
	}
	if err := json.Unmarshal(body, &rel); err != nil {
		return err
	}

	assetName := BinaryPath()
	var url string
	for _, a := range rel.Assets {
		if a.Name == assetName {
			url = a.BrowserDownloadURL
			break
		}
	}
	if url == "" {
		return fmt.Errorf("brak assetu %s dla platformy %s/%s", assetName, runtime.GOOS, runtime.GOARCH)
	}

	return downloadFile(ctx, url, BinaryFilePath())
}

// downloadFile pobiera url pod outPath ATOMOWO: najpierw do pliku *.tmp,
// po skończeniu rename — przerwane pobieranie nie zostawia uszkodzonego
// yt-dlp.exe, który silnik przy następnym starcie próbowałby odpalić.
func downloadFile(ctx context.Context, url, outPath string) error {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("pobieranie yt-dlp: %s", resp.Status)
	}

	tmpPath := outPath + ".tmp"
	out, err := os.Create(tmpPath)
	if err != nil {
		return err
	}

	_, copyErr := io.Copy(out, resp.Body)
	closeErr := out.Close()
	if copyErr != nil || closeErr != nil {
		os.Remove(tmpPath)
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	}

	if runtime.GOOS != "windows" {
		if err := os.Chmod(tmpPath, 0755); err != nil {
			os.Remove(tmpPath)
			return err
		}
	}

	// rename na Windows nie nadpisuje istniejącego pliku — usuń stary najpierw
	_ = os.Remove(outPath)
	if err := os.Rename(tmpPath, outPath); err != nil {
		os.Remove(tmpPath)
		return err
	}
	return nil
}

// EnsureFFmpeg sprawdza czy ffmpeg istnieje w PATH lub obok aplikacji.
func EnsureFFmpeg() (string, error) {
	if _, err := exec.LookPath("ffmpeg"); err == nil {
		return "ffmpeg", nil
	}
	// spróbuj obok binary
	exe, _ := os.Executable()
	dir := filepath.Dir(exe)
	candidate := filepath.Join(dir, "ffmpeg")
	if runtime.GOOS == "windows" {
		candidate += ".exe"
	}
	if _, err := os.Stat(candidate); err == nil {
		return candidate, nil
	}
	return "", fmt.Errorf("ffmpeg nie znaleziony – zainstaluj ffmpeg i dodaj do PATH lub obok aplikacji")
}
