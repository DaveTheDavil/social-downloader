package main

import (
	"bytes"
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"social-downloader-engine/internal/ads"
	"social-downloader-engine/internal/api"
	"social-downloader-engine/internal/license"
	"social-downloader-engine/internal/updater"
)

const (
	apiPort       = "8765"
	licenseFile   = "license.key"
	publicKeyFile = "public-key.pem"
	adsConfigFile = "ads-config.json"
	igCookieFile  = "ig-cookie.txt"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// -addr: klient iOS łączy się z PC przez LAN, więc silnik trzeba wystawić
	// poza localhost: `downloader.exe -addr :8765` = nasłuch 0.0.0.0:8765.
	// Bez flagi zachowanie bez zmian (127.0.0.1:8765) — tryb lokalny/badawczy.
	addrOverride := flag.String("addr", "", "adres nasłuchu, np. ':8765' (dostęp także z sieci lokalnej) — domyślnie 127.0.0.1:8765")
	flag.Parse()

	log.Println("[engine] uruchamianie silnika social-downloader...")

	// Wszystkie pliki danych rozwiązywane obok executable (NIE w bieżącym
	// katalogu roboczym) — silnik działa poprawnie też uruchomiony z innego
	// katalogu (Scheduler, skrót, terminal otworzony gdzie indziej).
	exePath, err := os.Executable()
	if err != nil {
		log.Fatalf("[engine] nie można ustalić ścieżki executable: %v", err)
	}
	engineDir := filepath.Dir(exePath)

	// 1. Sprawdź i ewentualnie zaktualizuj yt-dlp
	if err := updater.EnsureYtDlp(ctx); err != nil {
		log.Printf("[engine] ostrzeżenie: nie można zaktualizować yt-dlp: %v", err)
	}

	// 2. Wiąż licencję z Hardware ID
	hwid, err := license.MachineID()
	if err != nil {
		log.Fatalf("[engine] nie można wygenerować Hardware ID: %v", err)
	}
	log.Printf("[engine] Hardware ID: %s", hwid)

	pubKeyPEM, err := os.ReadFile(filepath.Join(engineDir, publicKeyFile))
	if err != nil {
		log.Fatalf("[engine] nie można wczytać klucza publicznego z %s: %v", publicKeyFile, err)
	}
	lic, err := license.Load(filepath.Join(engineDir, licenseFile), string(pubKeyPEM), hwid)
	if err != nil {
		log.Printf("[engine] brak ważnej licencji premium: %v (tryb darmowy z reklamami)", err)
	} else {
		log.Printf("[engine] licencja premium aktywna (wiązanie z HWID: %s)", lic.HardwareID)
	}

	// Wczytaj konfigurację reklam
	adsConfigPath := filepath.Join(engineDir, adsConfigFile)
	adCfg, err := ads.Load(adsConfigPath)
	if err != nil {
		log.Printf("[engine] ostrzeżenie: nie wczytano konfiguracji reklam: %v (brak reklam)", err)
		adCfg = &ads.Config{}
	}

	// Katalogi kreatyw reklam (podgląd + upload w panelu /ads-panel):
	// 1. adsimg obok silnika (samowystarczalny layout),
	// 2. ../app/AdsImg  — layout zainstalowany/paczka (kreatywy jada z klientem),
	// 3. ../windows/AdsImg — katalog dev-repo (rozwoj autora).
	adsImageDirs := []string{
		filepath.Join(engineDir, "adsimg"),
		filepath.Join(engineDir, "..", "app", "AdsImg"),
		filepath.Join(engineDir, "..", "windows", "AdsImg"),
	}

	ytDlpPath := filepath.Join(engineDir, updater.BinaryPath())

	// ffmpeg: PATH lub obok executable (wymagany do scalania wideo+dźwięk)
	ffmpegPath, ffmpegErr := updater.EnsureFFmpeg()
	if ffmpegErr != nil {
		log.Printf("[engine] ostrzeżenie: %v (pobieranie formatów łączonych może działać źle)", ffmpegErr)
	}

	// Sesja Instagram (cookie): opcjonalny ig-cookie.txt obok executable
	// (przycisk „Zapisz sesję" w kliencie zapisuje tutaj; edycja ręczna też
	// działa). UTF-8 bez BOM — PowerShell Set-Content potrafi go dodać,
	// wycinamy defensywnie. Czytane przy starcie, zmiany przez POST /ig-cookie.
	igCookiePath := filepath.Join(engineDir, igCookieFile)
	igCookie := ""
	if data, err := os.ReadFile(igCookiePath); err == nil {
		data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
		igCookie = strings.TrimSpace(string(data))
	}
	if igCookie != "" {
		log.Printf("[engine] sesja Instagram: wczytano %s", igCookieFile)
	}

	// 3. Uruchom serwer API (domyślnie localhost; -addr ':8765' wystawia LAN —
	// w takim trybie zaporowa Windows musi przepuszczać port 8765. Uwaga:
	// /ig-cookie i /file (pliki z całego katalogu domowego) są wtedy dostępne
	// dla dowolnego hosta w LAN, więc wystawiaj tylko zaufane sieci domowe.)
	addr := "127.0.0.1:" + apiPort
	if a := strings.TrimSpace(*addrOverride); a != "" {
		if !strings.Contains(a, ":") {
			addr = "0.0.0.0:" + a // samo "8765" też powinno działać intuicyjnie
		} else {
			addr = a
		}
	}
	srv := api.New(&api.Config{
		Addr:          addr,
		YtDlpPath:     ytDlpPath,
		FfmpegPath:    ffmpegPath,
		License:       lic,
		MaxYouTubeSec: 600, // 10 minut – hard limit, unikanie longplayów
		Ads:           adCfg,
		IgCookie:      igCookie,
		IgCookieFile:  igCookiePath,
		AdsFile:       adsConfigPath,
		AdsImageDirs:  adsImageDirs,
	})

	go func() {
		log.Printf("[engine] API nasłuchuje na %s", addr)
		log.Printf("[engine] panel reklamodawców: http://%s/ads-panel", addr)
		if err := srv.Start(); err != nil {
			log.Fatalf("[engine] błąd serwera API: %v", err)
		}
	}()

	// Czekaj na sygnał zakończenia
	<-ctx.Done()
	log.Println("[engine] zamykanie serwera...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("[engine] błąd podczas zamykania: %v", err)
	}
}
