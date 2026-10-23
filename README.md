# Social Downloader

Aplikacja zewnętrzna do pobierania filmów i zdjęć z mediów społecznościowych (Facebook, Instagram, TikTok, YouTube), zbudowana **od podstaw**, całkowicie niezależna od zewnętrznych serwerów. Dystrybuowana bezpośrednio (bez sklepów).

## Architektura

```
┌─────────────────────┐      ┌─────────────────────────┐
│  KLIENT NATYWNY     │      │     SILNIK POBIERANIA   │
│  (Swift iOS)        │─────▶│  (Go, lokalny)          │
│  (C# .NET Windows)  │      │  • wywołuje yt-dlp      │
│  UI + reklamy       │      │  • wywołuje FFmpeg      │
│  premium (jednoraz) │      │  • API lokalne HTTP     │
└─────────────────────┘      └─────────────────────────┘
                                   │
                    ┌──────────────┴──────────────┐
                    ▼                             ▼
              yt-dlp binary                   FFmpeg
              (osadzony w apce)               (osadzony w apce)
```

### Warstwy
| Warstwa | Technologia | Uwagi |
|-----------|-------------|-------|
| Silnik | Go | Lokalny serwer HTTP na `127.0.0.1:8765`, wywołuje `yt-dlp` i `FFmpeg` |
| Pobieranie | `yt-dlp` (binary) | Autoudostępniany z GitHub releases przy starcie |
| Przetwarzanie | FFmpeg | Scalanie wideo+dźwięku, konwersja formatów |
| iOS | Swift + SwiftUI | Natywny, od podstaw |
| Windows | C# + .NET 8 + WPF | Natywny, od podstaw |
| Reklamy | Lokalny `ads-config.json` | Baner + interstitial przed pobraniem (darmowa wersja) |
| Premium | Plik `license.key` (HWID) | Jednorazowe odblokowanie, usuwa reklamy, odblokowuje HD |

## Ograniczenia funkcjonalne (zgodnie z wymaganiami)

- **Obsługiwane platformy:** Facebook, Instagram, TikTok, YouTube
- **YouTube:** limit 10 minut na film (unikanie longplayów)
- **Treści prywatne:** blokowane (yt-dlp zwraca błąd → aplikacja blokuje pobranie)
- **Netflix / Disney+ / HBO:** NIE obsługiwane
- **Prywatne konta:** nie pobieramy treści z profili prywatnych
- **Reklamy:** darmowa wersja wyświetla baner reklamowy i interstitial przed pobraniem
- **Premium (jednorazowe):** usuwa reklamy, odblokowuje rozdzielczości powyżej 720p
- **Wiązanie z Hardware ID:** licencja powiązana z konkretnym urządzeniem

## Struktura projektu

```
social-downloader/
├── engine/                 # Silnik w Go
│   ├── main.go             # Punkt wejścia (autoupdate yt-dlp, HWID, licencja, API)
│   ├── ads-config.json     # Konfiguracja reklamodawców
│   ├── internal/
│   │   ├── ads/            # Model reklam, losowanie wagowane
│   │   ├── api/            # Serwer HTTP: /info, /download, /progress, /license, /ads
│   │   ├── downloader/     # Wywołanie yt-dlp (-J, pobieranie)
│   │   ├── license/        # HWID, walidacja licencji RSA (RSA + SHA-256)
│   │   └── updater/        # Sprawdzanie wersji yt-dlp na GitHub
│   └── go.mod
├── windows/                # Klient Windows (C# + .NET 8 + WPF)
│   ├── SocialDownloader.csproj
│   ├── App.xaml / App.xaml.cs
│   ├── MainWindow.xaml / .cs
│   ├── Models/             # VideoInfo, Format, AdUnit, LicenseInfo
│   ├── Services/           # EngineClient, LicenseService, AdService
│   ├── ViewModels/         # MainViewModel (logika aplikacji)
│   └── Views/              # AdInterstitial (okno reklamy przed pobraniem)
├── ios/                    # Klient iOS (Swift + SwiftUI)
│   ├── SocialDownloaderApp.swift
│   ├── Models/             # VideoInfo, Format, AdUnit, LicenseService
│   ├── Services/           # EngineClient (URLSession), AdService
│   ├── ViewModels/         # MainViewModel (ObservableObject)
│   └── Views/              # ContentView, AdBannerView, AdInterstitialView
└── README.md
```

## Komunikacja klient ↔ silnik

Serwer silnika nasłuchuje na `127.0.0.1:8765`:

| Endpoint | Metoda | Opis |
|----------|--------|------|
| `/healthz` | GET | Sprawdzenie czy silnik żyje |
| `/info?url=` | GET | Metadane filmu (tytuł, czas, autor, dostępne jakości) |
| `/download` | POST | `{url, format_id, output_path}` → uruchamia pobieranie |
| `/progress?job_id=` | GET | Postęp pobierania: `job_id`, `status`, `progress`, `error`, `output_path`, `url` |
| `/license` | GET | Stan licencji premium (`premium`, `features`, `hwid`) |
| `/ads?type=banner\|interstitial` | GET | Zwraca losową reklamę danego typu (jeśli nie premium) |

## Instalacja i budowanie

### Wymagania
- **Windows:** .NET 8 SDK (WPF), Go 1.22+ (do budowy silnika)
- **iOS:** Xcode 15+, macOS (do kompilacji pod iOS)
- **yt-dlp + FFmpeg:** silnik próbuje autoupdate'ować yt-dlp z GitHub; FFmpeg musi być dostępny w PATH lub obok aplikacji

### Silnik (Go)
```powershell
# W folderze engine/
$env:PATH = "C:\Users\DrKu5h\tools\go\bin;$env:PATH"
go build -o downloader.exe .
```

Uruchomiony silnik nasłuchuje na `127.0.0.1:8765`. Klienci łączą się z nim lokalnie.

### Windows (C# / WPF)
```powershell
# W folderze windows/
dotnet build SocialDownloader.csproj -c Release
dotnet publish SocialDownloader.csproj -c Release -o publish
```
Wynik: `publish/SocialDownloader.exe` + zależności. Dystrybuuj jako folder lub spakuj do instalatora (Inno Setup, WiX).

### iOS (Swift / SwiftUI)
1. Otwórz Xcode.
2. Utwórz nowy projekt **App** (SwiftUI, Swift, Interface: SwiftUI).
3. Skopiuj pliki z folderu `ios/` do projektu (zastępując domyślne `ContentView.swift`).
4. W `Signing & Capabilities` dodaj odpowiednie provisioning profile (dla dystrybucji bezpośredniej: Ad Hoc lub Enterprise).
5. Uruchom na fizycznym urządzeniu iOS (tryb deweloperski wymagany do instalacji poza App Store).
6. Upewnij się, że silnik Go jest uruchomiony i dostępny pod `http://127.0.0.1:8765` (lub zmień `baseURL` w `EngineClient.swift` na adres komputera w sieci).

> **Uwaga iOS:** Apple wymaga trybu deweloperskiego oraz podpisania aplikacji (nawet przy dystrybucji bezpośredniej). Uruchamianie obcych binary (yt-dlp/FFmpeg) na iOS jest ograniczone sandboxem — zalecane jest uruchomienie silnika na osobnym komputerze i połączenie przez sieć Wi-Fi, lub osadzenie skompilowanego pod iOS silnika jako framework (wymaga dodatkowej konfiguracji Xcode).

## Reklamy i premium

### Reklamy (darmowa wersja)
Plik `engine/ads-config.json` zawiera listę reklamodawców (banery + intersticale). Klienci pobierają konfigurację z endpointu `/ads`. Reklamy są losowane z uwzględnieniem wagi (`weight`).

### Premium (jednorazowe odblokowanie)
- Użytkownik kupuje klucz licencyjny (np. przez Twoją stronę z płatnościami).
- W aplikacji wpisuje klucz lub używa przycisku testowego.
- Klucz jest zapisywany w pliku `license.key` i powiązany z Hardware ID urządzenia.
- Po aktywacji: baner i interstitial znikają, odblokowuje się HD (>720p).

### System zarządzania reklamodawcami
Edytuj `engine/ads-config.json` aby dodawać/usuwać reklamodawców. Każda reklama ma:
- `id`, `advertiser`, `title`, `description`, `url` (link docelowy)
- `image` (ścieżka lub URL do grafiki)
- `type` (`banner` lub `interstitial`)
- `weight` (waga w losowaniu)
- `seconds` (czas wyświetlania interstitial)

## ⚠️ Disclaimer prawny

```
Aplikacja służy do pobierania wyłącznie treści, na które masz prawa
lub które są w domenie publicznej. Użytkownik ponosi pełną
odpowiedzialność za przestrzeganie regulaminów platform i praw autorskich.
```

Pobieranie treści z Facebook, Instagram, TikTok i YouTube może naruszać ich regulaminy usług (ToS). Aplikacja nie pobiera treści z kont prywatnych ani chronionych DRM (Netflix, Disney+, HBO). Autor aplikacji nie ponosi odpowiedzialności za niewłaściwe użycie.

## Następne kroki (rozwojowe)

1. Dodać podpisywanie binarek (code signing) dla Windows/iOS.
2. Zaimplementować prawdziwą walidację RSA licencji — zrobione (RSA + SHA-256, PKCS#1 v1.5).
3. Dodać rzeczywisty postęp pobierania (yt-dlp `--progress` lub `--newline`).
4. Dodać opcję wyboru folderu docelowego.
5. Dodać historię pobrań i galerię.
6. Dodać panel webowy do zarządzania reklamodawcami (hosting własny).
