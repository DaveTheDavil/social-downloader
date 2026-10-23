# README-iOS — klient iOS „Social Downloader” (LAN)

Klient SwiftUI (iOS 16+) dla silnika Go na PC. Łączenie: **LAN bezpośrednio**
(`http://<IP-PC>:8765`). Budowa bez Maca:

```
GitHub Actions (macos) → niepodpisana .ipa → Sideloadly (Windows) → iPhone
```

## Struktura

```
ios/project.yml                  # XcodeGen; projekt generowany na runnerze
ios/Sources/SocialDownloaderApp.swift   # @main, zakładki, AppStore
ios/Sources/EngineAPI.swift             # klient HTTP (async/await, Codable)
ios/Sources/Models.swift                # modele JSON silnika
ios/Sources/MainScreen.swift            # Pobieraj: analiza, jakość, seria, batch
ios/Sources/DownloadsScreen.swift       # Pobrania: Documents + ShareLink
ios/Sources/SettingsScreen.swift        # Ustawienia: adres + test
ios/Sources/Adviews.swift               # baner tekstowy + interstitial
```

Niecommitować (generowane): `SocialDownloader.xcodeproj/`, `Info.plist`,
`build/`, `*.ipa`.

## Kontrakt silnika

| Endpoint | Metoda | Rola |
|----------|--------|------|
| `/healthz` | GET | test połączenia |
| `/info?url=` | GET | analiza: tytuł, autor, quality picker, entries |
| `/download` | POST | `{url, format_id, output_path, item}`; `item` 1-based |
| `/progress?job_id=` | GET | poll co 1 s |
| `/file?job_id=` | GET | plik — dopiero po `completed` |
| `/ads?type=banner\|interstitial` | GET | darmowe: reklama; premium: `ad=null` |
| `/ig-cookie` | GET/POST | `{cookie}` — zapis sesji Instagram |

Uwagi:
- `output_path` względny: `iOS-Downloads/<tytuł>[_<index>].<ext>`.
  Silnik zapisuje plik obok siebie (względem CWD), apka pobiera go przez `/file`
  i zapisuje w Documents.
- `AdUnit.image` to ścieżka na PC — iOS jej nie ładuje; reklamy tekstowe.
- Adres silnika: UserDefaults klucz `engineURL`.

## 5 kroków dla użytkownika

1. **Repo na GitHub** — utwórz **publiczne** repo (darmowy macos runner).
   Push wszystkiego poza `.gitignore` (sekrety: `keys/`, `engine/license.key`,
   `engine/ig-cookie.txt`, `*.exe`, `*.ipa`, `.xcodeproj`, `ios/build/`).
2. **Workflow** — Actions → „iOS build” → Run (albo push w `ios/**`).
   Dostaniesz artefakt `social-downloader-ipa` (plik `.ipa`).
3. **Sideloadly** — zainstaluj: iTunes 64-bit + iCloud (apple.com, NIE Microsoft
   Store) + Sideloadly (sideloadly.io). iPhone z kablem, „Zaufaj komputerowi”.
4. **Instalacja** — w Sideloadly: przeciągnij `.ipa`, podaj darmowe Apple ID,
   Start. Na iPhonie: Ustawienia → Ogólne → VPN i zarządzanie urządzeniami →
   zaufaj profilowi. Ustawienia → Prywatność → **Tryb dewelopera** (iOS 16+).
5. **Silnik + LAN** — na PC w katalogu `engine` uruchom
   `downloader.exe -addr :8765`; zezwól zaporze port 8765 (TCP, sieci prywatne).
   IP komputera: `ipconfig` (IPv4). W apce: Ustawienia →
   `http://<IP-PC>:8765` → „Zapisz” → „Testuj połączenie” → status zielony.

Sesja Instagram (pełne serie): panel u dołu zakładki „Pobieraj”.

## Checklist pierwszego builda na runnerze

- `xcodegen generate` przed `xcodebuild` (workflow właśnie tak robi).
- Scheme = `SocialDownloader` (jawna w `project.yml`).
- Code signing: `CODE_SIGNING_ALLOWED=NO` → `.app` w Release-iphoneos → zip.
- Info.plist generowany z props (`NSAllowsArbitraryLoads` = HTTP LAN działa).
- API: tylko iOS 16/15 (`NavigationStack`, `ShareLink`, `presentationDetents`,
  `.task`). Bez iOS 17+ i bez `@Observable`.
- Typowe błędy: brak scheme (xcodegen nie odpalony), ATS przy HTTP (Info.plist
  nie właściwy), błąd Codable (pole zmienione w silniku — popraw `Models.swift`).
- Czas joba: 2–4 min (limits GitHub OK).

## Ryzyka po instalacji

- IP PC w LAN może się zmienić (DHCP) — wystarczy poprawka w Ustawieniach.
- Zaporę trzeba pozwolić ponownie, gdy sieć Wi-Fi się zmieni.
- Seria IG bez sesji: tylko pierwsza pozycja (limit Instagramu).
- Darmowe Apple ID: apka wymaga re-sideload co 7 dni.
- Uruchamiaj silnik z jego katalogu — `output_path` jest względem CWD.
