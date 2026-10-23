# Notatki sesji — Social Media Downloader

## 📦 Pakiet instalacyjny Windows (2026-10-23)

- **Zbudowano przenośny pakiet ZIP na drugi komputer (Win10/11 x64):** `C:\Users\DrKu5h\Desktop\INSTALKA\SocialDownloader-1.0.0-win64.zip` (self-contained klient WPF + silnik z yt-dlp/ffmpeg + README/START.bat; sekret-gate czyści: bez license.key/ig-cookie/klucza prywatnego). Uruchomienie na drugim PC: rozpakować folder → START.bat (silnik hidden @127.0.0.1:8765 + klient). Klient publish był **self-contained** (`--self-contained true -p:DebugType=none -p:DebugSymbols=false` → `windows/publish`, hostfxr.dll wbudowany — nic nie instalować).
- **Regeneracja pakietu:** (1) `engine/`: `go build -o downloader.exe .`; (2) `windows/`: `dotnet publish SocialDownloader.csproj -c Release -r win-x64 --self-contained true -p:DebugType=none -p:DebugSymbols=false -o publish`; (3) `powershell -File tools\package-windows.ps1 [-Version x.y.z]` — skrypt składa staging na `Desktop\INSTALKA\`, skanuje sekret-gate (license.key/ig-cookie*/private*/*.pem≠public/keys/obj/.git*/źródła .go,.cs,.xaml/.exe-poza-whitelistą → FAIL, nie zipuje), zipkuje + kopiuje README-INSTALKA.txt + wypluwa SHA256. Skrypt NIC nie kompiluje.
- **Weryfikacja pakietu (na 8766):** healthz ok; `/info` (IG post) → format `img` 1350p; POST `/download` → `completed` 100%; `out.jpg` 79 934 B = `GET /file` round-trip (HTTP 200, ten sam rozmiar); `/license` premium=false; `/ads?type=banner` → banner zwrócony. `START.bat` przetestowany (silnik startuje ukryty z logiem `%TEMP%\sd-engine.log`, klient wstaje); yt-dlp w pakiecie 2026.10.21 (self-update działa z GitHuba, offline zostaje wersja z pakietu).
- **Cechy na drugim PC:** tryb darmowy (reklamy, HD ≤720p) — license.key jest HWID-bound i nie jest dołączane przez design;`ig-cookie.txt` opcjonalne (klient „Zapisz sesję"); silnik nasłuchuje wyłącznie 127.0.0.1:8765.

## Stan: SILNIK ZOPTYMALIZOWANY PO PEŁNEJ ANALIZIE + WERYFIKACJA NA ŻYWO (2026-10-23)

## ✅ Poprawki krytyczne (z raportu analizy, po optymalizacji)

1. `getJob` zwraca kopię Job (nie wskaźnik) — koniec data race między goroutinem pobierania a `/progress`
2. `http.Server` tworzony w `New()` — pole `s.srv` nie jest już nadpisywane po starcie (race Start/Shutdown)
3. Walidacja `url` — wymagane http(s):// (blokuje flagi yt-dlp typu `--exec`)
4. Walidacja `format_id` — whitelist `^[A-Za-z0-9_+][A-Za-z0-9_.-]{0,63}$` (blokuje iniekcje flag)
5. Whitelist `output_path` — tylko katalog domowy użytkownika (blokuje nadpisanie np. C:\Windows)
6. Fail-closed premium: `/download` zawsze robi własną ekstrakcję; bramka HD pokrywa też formaty łączone (`135+140`) i selektory (`bestvideo` → 403 dla darmowych)
7. Limit 10 min YouTube egzekwowany też w `/download` (klient pomijający `/info` nie obchodzi limitu)
8. wykrywanie privatnych treści z stderr yt-dlp (`ErrPrivate` → 403); usunięta fałszywa heurystyka po tytule
9. `exec.CommandContext` z timeoutami: info 3 min, download 30 min, `--version` 15 s (zero procesów-zombiaków)
10. Wszystkie pliki danych (`license.key`, `public-key.pem`, `ads-config.json`) i yt-dlp rozwiązywane obok executable (niezależnie od katalogu roboczego)

## 🔒 Bezpieczeństwo

- `private-key.pem` WYJĘTY z `engine/` → `social-downloader/keys/` (klucz obok silnika pozwalałby każdemu lokalnemu procesowi podpisać sobie licencję premium)
- `.gitignore` nowy (klucze, binarne, artefakty testowe)
- `cmd/signlic/` — narzędzie wydawcy: podpisuje licencję (hwid + features, PKCS#1 v1.5 + SHA-256), zgodnie z weryfikacją w `internal/license`
  - użycie: `go run .\cmd\signlic -key ..\keys\private-key.pem -hwid "win:..." -features "hd,nowatermark" -out license.key`
- Poprawka `handleAds`: premium = `ad:null` (wcześniej reklamy szły także dla premium — niezgodny ze specyfikacją)

## ✅ Tryb premium zweryfikowany na żywo

- `signlic` → `engine/license.key` (hwid `win:00c406ac-…:sHELL`, features `hd,nowatermark`)
- Restart silnika → log "licencja premium aktywna (wiązanie z HWID: …)"
- `GET /license` → `{"features":["hd","nowatermark"],"hwid":"win:…","premium":true}`
- `GET /ads?type=interstitial` → `{"ad":null,"premium":true}` (brak reklam przy premium)
- `GET /info` → `premium:true, features:"hd,nowatermark"`
- **POST /download `137+140` (1080p >720p)** → completed 100%, `test_hd.mp4` **80.5 MB** — bramka HD otwarta licencją (darmowa wersja: 403)
- Lekcja z walidacji: pierwsza regexpa format_id odrzucała `137+140` (dozwalała `+` tylko na pierwszym znaku) → klasa rozszerzona do `[A-Za-z0-9_.+-]` w środku, start nadal bez `-`

## ⚡ Optymalizacja / sprzątanie

- `installedVersion()` działa przez pełną ścieżkę (`BinaryFilePath()`) — koniec wymuszonych re-pobrań yt-dlp przy każdym starcie (log: `zainstalowana wersja="2026.10.21"`)
- Atomowe pobieranie yt-dlp (`.tmp` + rename; przerwany download nie zostawia zepsutego exe)
- Janitor zadań (retencja 2 h; mapa jobs nie rośnie w nieskończoność)
- `writeJSON` → JSON Encoder
- Martwy kod usunięty: `license.Save`, `ads.HasAny` (razem z TestHasAny), `downloader.RunDownload`, stałe `ytDlpBinary`/`ffmpegBinary`/`checkInterval`, uproszczony `BinaryPath()`
- `cmd/probe/` usunięty
- gofmt + go vet + `go test ./...` → wszystko OK; build OK

## Weryfikacja na żywo po optymalizacji (tylko curl.exe)

- `format_id: "--exec whoami"` → **400** "nieprawidłowy format_id" (iniekcja zablokowana)
- `url: ftp://…` → **400** "url musi zaczynać się od http:// lub https://"
- `output_path: C:\Windows\evil` → **400** "ścieżka wyjściowa musi być w katalogu domowym"
- `format_id: bestvideo` bez licencji → **403** "wymaga wersji premium" (fail-closed)
- `/info` film niedostępny → **403** "treść jest prywatna lub niedostępna" (wcześniej mylnie 502)
- `/info` normalny film → **200**; tytuł, 213 s, jakości 144–2160, premium False
- `/ads?type=interstitial` → reklama z configu (konfiguracja wczytana z katalogu exe)
- HWID z rejestru: `win:00c406ac-…:sHELL` (spójny z klientem C#)

## Stan ogólny (testy E2E z wcześniejszej sesji)

- Silnik Go nasłuchuje `127.0.0.1:8765`
- Autoupdate yt-dlp: `yt-dlp.exe` 17,4 MB obok exe
- FFmpeg lokalny: `engine/ffmpeg.exe` (wersja N-127941); `main.go` rozwiązuje ścieżkę przez `EnsureFFmpeg()`
- E2E pobranie progressive: format 603 (144p) → `test_dl.mp4` 1514 KB
- E2E scalanie: format `135+140` → `test_merge.mp4` 16.8 MB

## ⚙️ Klient Windows C# — kompilacja i uruchomienie (2026-10-23)

- `.NET 8 SDK 8.0.426` zainstalowany przez użytkownika w `C:\Program Files (x86)\dotnet` (x86 build OK dla WPF); do builda:
  `$env:DOTNET_ROOT = "C:\Program Files (x86)\dotnet"; $env:PATH = "$env:DOTNET_ROOT;$env:PATH"` (PATH nie łapie się w starych procesach)
- `dotnet build -c Release` → `windows\bin\Release\net8.0-windows\SocialDownloader.exe` (118 KB) + ads-config.json
- **Poprawki przed pierwszym buildem:**
  - App.xaml: `ControlTemplate` bez opakowania `<Setter.Value>` → MC3059
  - MainWindow.xaml + AdInterstitial.xaml: błędny URI `xmlns:mc` (`schemas.microsoft.com` → właściwe `schemas.openxmlformats.org`) → MC3072
  - csproj: brak `ImplicitUsings` (kod pisany z założeniem włączenia)
  - `UseWindowsForms=true` usuwa `System.Net.Http` z implicit usings → dodany explicite w EngineClient
  - global using `System.Windows.Forms` (z WinForms) koliduje z WPF → `App : System.Windows.Application`, `MessageBox.Show` → `System.Windows.MessageBox`
  - duplikat klasy `Format` (VideoInfo.cs + Format.cs) — usunięty z VideoInfo
  - MainViewModel: dodane property `InterstitialAd` (było tylko pole), `CancellationToken.Cancel()` → `return` z pętli (CT nie ma Cancel)
  - `Window.ShowDialog(this)` → `{ Owner = this }` + `ShowDialog()` (WPF nie ma przeciążenia z argumentem)
  - **CRASH at startup (NRE)**: hooki eventów VM w `MainWindow..ctor()` przy null DataContext → przeniesione do `MainWindow.HookViewModelEvents()` wywoływanego z `App.OnStartup` PO przypisaniu DataContext
- Klient uruchomiony: proces stabilny (PID 10004, ~89 MB), silnik w tle działa (healthz OK)
- **Bugi GUI po pierwszym resecie (2026-10-23) — root cause wspólne: kontrakt JSON snake_case ≠ C# PascalCase.**
  `ReadFromJsonAsync` mapuje case-insensitive, ale NIE mapuje podkreślników → `format_id`,`job_id`,`output_path` wchodziły pusto:
  - dodane `[JsonPropertyName("…")]` do wszystkich modeli (VideoInfo, Format, AdUnit/AdResponse, LicenseInfo, DownloadStatus/DownloadResponse) — kontrakt: `server.go` + `ydlp.go` + `ads.go`
  - **ComboBox jakości pusty**: `Qualities` w VM to computed property (`VideoInfo?.Qualities`), a setter `VideoInfo` nie odpalał `OnPropertyChanged(Qualities)` → ComboBox trzymał stary pusty obiekt listy. Fix: notify `Qualities`/`HasQualities` + reset `SelectedQuality`/`SelectedFormat` przy nowej analizie
  - **JobId pusty** (bez tej naprawy pobieranie zawsze pokazywałoby "Brak odpowiedzi")
  - panel aktywacji premium zawsze widoczny → `BooleanToVisibilityInverseConverter` + `Visibility={Binding IsPremium}` (chowa się, gdy silnik potwierdza premium)
  - domyślny folder: `%USERPROFILE%\Downloads\SocialDownloader` (create-on-start; "Pobierz plik" aktywuje się bez ręcznego wybierania katalogu)
  - UWAGA: IG reel zwraca DUR=0 i qualities [358, 480] — to REALNE rozdzielczości danego pliku, nie bug; formaty `1`,`2`,`3` mają puste vcodec/i height=0 (pomijane przez filtr)

## Posty IG ze zdjęciami i seriami (2026-10-23, retest 2)

- Root cause: yt-dlp NIE OBSŁUGUJE zdjęć z IG — każdy post /p/ kończy się "No video formats found!" (weryfikacja: `engine\yt-dlp.exe -J` na post DexBwdADpTo).
- Anonimowe ścieżki IG (przetestowane 2026-10-23): DZIAŁA TYLKO `/p/<kod>/media/?size=l` → 302 na pełny plik CDN (dla serii = PIERWSZY element). Martwe: api/v1 (www i i.instagram, 2 app-id, desktop+mobile UA), GraphQL doc_id 884575858208211 (403), __a=1&__d=dis (404), embed/captioned z random ig_did (login redirect), HTML posta (JS-shell bez og:image). Pełna lista serii wymaga sesji IG → feature na później (cookie).
- Silnik: nowy `internal/downloader/insta.go` — ErrNoVideoFormats, InstagramShortcode, ResolveInstagramPhoto (resolver + parsowanie wymiarów z nagłówka SOF JPEG), RunDirectDownloadWithProgress (HTTP wprost, atomowo .part→rename, postęp z Content-Length). RunInfo mapuje stderr "no video formats found" na ErrNoVideoFormats. Kontrakt v2: /info + is_playlist/entries/playlist_note (stare klucze nietknięte). /download: fallback resolver fail-closed, obrazy zwolnione z bramki HD, pobieranie rozgałęzione direct/ytdlp. LIVE-TEST: zdjęcie 1080x1350, download 79 934 B OK; reels bez regresji (358/480).
- Klient: VideoInfo+EntryInfo (JsonPropertyName), EngineClient timeout 300 s + czytanie body błędów ({"error"}), VM przepisany na pozycje: Items ObservableCollection DownloadItem (checkboxy), BuildItems (entries lub pseudo-entry pojedynczego), RunBatchAsync (sekwencyjnie, interstitial raz przed partią), ResolveForItem (obraz=img, wideo=zgodna wysokość z SelectedQuality), PremiumLabel Tak/Nie, reset stale danych przy analizie. XAML: nowy wiersz listy (Grid.Row 5), nota serii, przerysowane wiersze (baner *, premium, statusbar), meta-grid jako StackPanle (koniec nakładania "Czas:379 s LastSummerian").

### Sesja IG (cookie) — pełne serie (2026-10-23, wdrożone)

- Silnik: endpoint `GET/POST /ig-cookie` (GET → tylko `configured` bool, NIGDY wartość cookie; POST `{"cookie":"..."}` → zapis `engine/ig-cookie.txt` UTF-8 bez BOM + podmiana w pamięci pod igMu; pusty string kasuje plik i czyści sesję). Cookie wczytywany przy starcie z exe-dir (BOM wycinany defensywnie). `Config.IgCookie/IgCookieFile`.
- Silnik: `ResolveInstagramPost()` w `insta.go` — `GET https://www.instagram.com/api/v1/media/<kod>/info/` (Cookie + `x-ig-app-id: 936619743392459` + browserUA, timeout 30 s, CheckRedirect ≠ `/accounts/login` → `ErrSessionInvalid`); parsuje `items[0]`: media_type 8=karuzela (każdy `carousel_media` → PostItem), 1=zdjęcie (największy candidate z image_versions2), 2=wideo (największy wariant video_versions, `video_duration`); zwraca `[]PostItem{Index,ID,Title,IsImage,Duration,W,H,URL,Ext}` — formaty syntetyczne buduje warstwa api (`entriesFromPostItems`: `img` VCodec=image / `vid` h264+aac).
- Kolejność fallbacków /info i /download: yt-dlp → ErrNoVideoFormats && shortcode → sesyjny resolver (aktywna sesja: is_playlist=true, pełna lista entries, nota „Sesja IG aktywna — wszystkie pozycje dostępne.") → anonimowy (nota „Sesja IG ustawiona, ale wygasła/nieaktywna — odśwież cookie" po błędzie sesyjnym / zwykła nota o anonimowym elemencie 1). `/download` przyjął `{"item": N}` (1-based, 0/brak=1): ścieżka sesyjna waliduje `1<=item<=len(items)` i format img/vid zgodny z typem elementu, `RunDirectDownloadWithProgress(it.URL)`; anonimowo item=1 albo 400 „pełna lista serii wymaga sesji IG"; reels/vide yt-dlp bez zmian. **Bramka HD ograniczona do ścieżki yt-dlp (`directURL==""`) — sesyjne wideo >720p nie podlega blokadzie 403.**
- Klient: EngineClient `Get/SetIgCookieConfiguredAsync` + `item` w /download body; VM `IgCookieInput`/`IgSessionLabel`/`SaveIgCookieCommand` (cookie czyśczone z pola po zapisie, never logging), `RefreshIgSessionAsync` przy starcie, `ResolveForItem` zwraca pojedynczy format `img`/`vid` W PROSTO, bramka HD klienta omija formaty wprost; **naprawiony bug overwrite: `RunBatchAsync` przy batch>1 dokleja `_{i+1:D2}` do nazw plików** i przekazuje `it.Index` jako `item`. XAML: wiersz 8 = panel sesji IG (tekst+Zapisz sesję+label bursztynowy), premium 9, statusbar 10. Live-test silnika: fake cookie → graceful fallback anonimowy z notą (200), czyszczenie → configured=false, download img item=1 OK (79 934 B), item=2 anonimowo → 400, reels [358,480] bez regresji; buildy 0 errors (gofmt+vet+testy pass). Weryfikacja z PRAWDZIWYM cookie zostaje użytkownikowi. Przyszłość: `ig-cookie.txt` zapisywany otwartym tekstem — docelowo DPAPI.


## iOS klient (LAN, SwiftUI) — 2026-10-24

**Co powstało** (katalog `ios/`, od zera; stary szkic-archiwum → `tools/ios-draft-20261022-archiwum/`):
- `Sources/` SwiftUI (iOS 16+, bez API 17+ i bez `@Observable`): `SocialDownloaderApp` ({@main} + TabView Pobieraj/Pobrania/Ustawienia + `AppStore`), `EngineAPI` (async/await, `convertFromSnakeCase`, sesja API timeout 210 s bo /info do 3 min, osobna sesja dla `/file`), `Models`, `MainScreen` (analiza → karta wyniku → quality picker (domyślnie max) → lista pozycji z Toggle zbiorcza dla serii → **batch sekwencyjny**: POST /download (`item` 1-based) → poll /progress 1 s → GET /file → zapis do Documents), `DownloadsScreen` (Documents + ShareLink), `Settings` (adres silnika w UserDefaults `engineURL` + „Testuj połączenie"), `Adviews` (żółt\u0069 pasek banera; interstitial sheet z odliczaniem `ad.seconds`, „Pomiń" **aktywyne** po odliczeniu; `AdUnit.image` NIE ładowany na iOS — to ścieżka PC).
- `project.yml` (XcodeGen): target `SocialDownloader`, iOS 16.0, bundle `pl.thedavil.SocialDownloader`, Info.plist z props: `NSAllowsArbitraryLoads: true` (HTTP LAN), `UIFileSharingEnabled` + `LSSupportsOpeningDocumentsInPlace` (Documents w aplikacji Pliki), bez entitlements; scheme jawny. `.xcodeproj` NIE commitowany.
- `.github/workflows/ios.yml`: macos-latest → `brew install xcodegen` → `xcodegen generate` (w `ios/`) → `xcodebuild … CODE_SIGNING_ALLOWED=NO` → Payload → `.ipa` → artefakt `social-downloader-ipa`. Trigger: dispatch + push (`ios/**`).
- `ios/README-iOS.md`: 5 kroków (repo publiczne → workflow → artefakt → Sideloadly + iTunes + iCloud → zaufanie profilu/Tryb dewelopera → silnik `-addr :8765` + zapora + IP z `ipconfig`) oraz checklist pierwszego builda (co może paść).
- **Repo gitowe utworzone (2026-10-23)**: `git init -b main` + initial commit `368a159` (55 plików: engine 13, windows 18, ios 9, tools 11, `.github` 1) z lokalnym `user.name=DrKu5h` / `user.email=drku5h@users.noreply.github.com` (global nie był ustawiony). Gate „no secrets" **PASSED**: pem / license.key / keys/ / ig-cookie.txt / .env / exe / dll / ipa — zero w indeksie (build-artefakty `windows/obj/**` untracked + `*.pdb` dopisane do .gitignore). Push: `gh` niedostępny na maszynie → root projektu ma `PUSH-TO-GITHUB.bat`; co dalej: (1) utwórz PUBLICZNE repo `social-downloader` na github.com/new, (2) podmień `<TWOJ-LOGIN>` w batcie i uruchom go, (3) workflow `ios.yml` odpali się AUTOMATYCZNIE na pushu (paths zawiera `ios/**`, initial commit już je zawiera) → `https://github.com/<TWOJ-LOGIN>/social-downloader/actions/workflows/ios.yml` → artefakt `social-downloader-ipa`.

**Zmiany w silniku (mały dodatek na koniec endpointu /file):**
- **`GET /file?job_id=`** w `internal/api/server.go`: fail-closed narastająco — job istnieje (404), status `completed` (409 dla started/running/failed z treścią błędu), `isAllowedOutputPath` ponownဆ sprawdzany (403), plik istnieją i nie-katalog (404), potem `http.ServeContent`ranges; Content-Type z rozszerzenia (mapa own-fallback + `mime.TypeByExtension`), `Content-Disposition: attachment` via `mime.FormatMediaType` (RFC 5987 UTF-8 automatycznie). Two implying: `/file` wypuści plik TYLKO po zakończeniu joba — nie wypuszcza półnomu.
- **`main.go`: flaga `-addr`** — silnik był sztywno `127.0.0.1:8765`, teraz: bez flagi bez zmian; `downloader.exe -addr :8765` = nasłuch 0.0.0.0 (LAN dla iPhone). Uwaga bezpieczeństwa: przy `-addr` CAŁE API (łącznie z POST /ig-cookie i /file do całego home) jest dostępne w sieci — tylko zaufane Wi-Fi.
- Build + gofmt + `go vet ./...` clean; silnik zrestartowany, `/healthz` OK.

**Navarywanые żywe ponejmu testy `/file` (curl.exe):**
- pozytywny: job IG photo (пост DexBwdADpTo, format `img`) → completed → `/file` → **HTTP 200, size 79 934 B, Content-Type image/jpeg**; pierwsze 64 B = oryginał na dysku.
- negatywne: brak `job_id` → 400; nieistniejący job → 404; job w stanie `running` → **409**; job `failed` → 409 „pobieranie nieudane: …".
- Flaga `-addr :8790` (test izolowany): śluch na `:::8790` (dual-stack, IPv4-mapped) + healthz OK.
- OT-check: YouTube pobranie failju w TYP Środowisko (yt-dlp bez deno/JS runtime → brak formatu 18) — pre-kontekst silnika, Ivanowi wykopaliśmy przy testach; nie dotyczy `/file`.

**Uwagina(domowanie wersowania) klienca:**
- **`output_path` wysyła względny** (`iOS-Downloads/<tytuł>_<index>.<ext>`): silnik rez現uje względem SWOOM CWD (korporator z katalogu silnika; w README uwaga "uruchamiaj silnik z jego katalogu"). Nie domyślamy się PC-гo `%USERPROFILE%` z poziomu telefonu.
- Baner/interstitial: apka po prostu woła `/ads`; premium zwróci `ad:null` → nic się nie wyświetli (dotyczy też /info.premium). Post.
- Seria IG bez sesji: первая pozycja — bez chanżiki kodu, jest to kontrakt silnika.

## Pozostałe do zrobienia

1. Windows klient C#: **SKOMPILOWANY i URUCHOMIONY** (patrz sekcja wyżej) — została weryfikacja UX przez użytkownika
2. iOS klient Swift: **kod + workflow gotowe** (patrz sekcja „iOS klient (LAN)") — została akcja użytkownika: publiczne repo → Actions → artefakt `.ipa` → Sideloadly, oraz live-test z telefonu
3. Produkcja: `keys/private-key.pem` u wydawcy, `public-key.pem` w aplikacji (narzędzie `cmd/signlic` gotowe)
4. Brak gcc/cgo — `go build -race` niemożliwy; poprawki race zrobione ręcznie + `go vet`
5. Sprzątanie testów: `test_dl.mp4`, `test_merge.mp4`, `test_hd.mp4` w `%TEMP%\opencode\`
6. `LicenseService.GenerateTestKey()` pisze FEŁSZYWY podpis ("signed-test", bez RSA) — silnik go ODRZUCI; przycisk "Aktywuj premium" w kliencie wymaga prawdziwego `signlic` → license.key

## Kontrakt API (bez zmian)

```
GET /healthz → {"status":"ok"}
GET /info?url= → {id,title,duration,uploader,qualities[],formats[],premium,features[],hwid}
POST /download {url, format_id, output_path} → {job_id, status}
GET /progress?job_id= → {job_id,status,progress,error,output_path,url}
GET /license → {premium,hwid,features[]}
GET /ads?type=banner|interstitial → {type,ad,premium}
```

Nowe kody błędów: niepoprawne `url`/`format_id`/`output_path` → 400; film prywatny/niedostępny → 403; HD >720p bez premium → 403; film za długi → 403; inne błędy yt-dlp → 502.

## Pliki kluczowe

- `engine/main.go` — start, ścieżki exe-dir, HWID, licencja, API
- `engine/internal/api/server.go` — endpointy + walidacje + janitor
- `engine/internal/downloader/ydlp.go` — ctx + stderr-detection + postęp
- `engine/internal/license/license.go` — HWID (rejestr) + RSA weryfikacja
- `engine/internal/updater/updater.go` — BinaryFilePath, atomowy download
- `keys/private-key.pem` — klucz prywatny (u wydawcy, nie w engine/)
- `windows/`, `ios/` — klienci (build zablokowany brakiem SDK/macOS)
