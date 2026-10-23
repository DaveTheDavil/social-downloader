# Budowa instalatora Windows — Social Downloader

Ten dokument opisuje jak przygotować instalator dla klienta Windows (WPF, .NET 8) znajdującego się w folderze `windows/`.

## Wymagania wstępne

- **.NET 8 SDK** — zainstalowany na maszynie budującej (`dotnet --version` musi zwracać wersję 8.x).
- Windows 10/11, architektura docelowa `win-x64` (lub `win-x86`).
- Pliki zależne do dołączenia:
  - `yt-dlp.exe` — silnik pobierania treści (https://github.com/yt-dlp/yt-dlp/releases).
  - `ffmpeg.exe` — wymagane przez yt-dlp do łączenia strumieni wideo/audio (https://ffmpeg.org/download.html, wersja "essentials" build dla Windows).
  - (opcjonalnie) silnik Go uruchomiany lokalnie na `127.0.0.1:8765` — klient WPF komunikuje się z nim przez HTTP (`EngineClient`).

## Szybki start — skrypt automatyzujący

W folderze `windows/` uruchom:

```powershell
.\build-windows.ps1
```

Skrypt:
1. sprawdza, czy .NET 8 SDK jest zainstalowane (`dotnet --version`),
2. wykonuje `dotnet build SocialDownloader.csproj -c Release`,
3. wykonuje `dotnet publish SocialDownloader.csproj -c Release -r win-x64 --self-contained false -o publish/`,
4. wypisuje instrukcję ręcznego dodania `yt-dlp.exe` i `ffmpeg.exe` do `publish/`,
5. wypisuje instrukcję budowy instalatora Inno Setup / WiX (nie tworzy instalatora).

Na końcu skrypt wypisuje podsumowanie.

Opcjonalne parametry:
- `-SkipBuild` — pomiń krok budowy.
- `-SkipPublish` — pomiń krok publikacji.

## Krok 1 — Opublikuj aplikację (ręcznie, jeśli nie używasz skryptu)

```powershell
dotnet publish SocialDownloader.csproj -c Release -r win-x64 --self-contained false
```

- `--self-contained false` wymaga, by na maszynie docelowej był zainstalowany .NET 8 Runtime.
- Jeśli chcesz jednoexe bez zależności od runtime, użyj `--self-contained true` (większy plik, nie wymaga .NET na docelowej maszynie).
- Wynik trafia do `bin\Release\net8.0-windows\win-x64\publish\` (lub do `publish/` przy użyciu `-o publish`).

## Krok 2 — Dołącz pliki zewnętrzne

Do folderu `publish\` (lub podfolderu `tools\`) skopiuj:

```
publish\
├── SocialDownloader.exe
├── SocialDownloader.dll
├── ... (pozostałe wyjście publish)
├── yt-dlp.exe
├── ffmpeg.exe
└── ads-config.json
```

Klient oczekuje, że silnik Go (osobny proces) słucha na `127.0.0.1:8765`. Instalator może opcjonalnie dołączyć wykonywalny silnika i uruchamiać go jako proces towarzyszący, ale to poza zakresem klienta WPF.

## Krok 3 — Zbuduj instalator

### Opcja A: Inno Setup (zalecane, najprostsze)

Zainstaluj Inno Setup (https://jrsoftware.org/is.php) i utwórz skrypt np. `SocialDownloader.iss`:

```iss
#define MyAppName "Social Downloader"
#define MyAppVersion "1.0"
#define MyAppPublisher "Twój Autor"
#define MyAppExe "SocialDownloader.exe"

[Setup]
AppId={{...wygeneruj nowe GUID...}}
AppName={#MyAppName}
AppVersion={#MyAppVersion}
AppPublisher={#MyAppPublisher}
DefaultDirName={pf}\{#MyAppName}
DefaultGroupName={#MyAppName}
AllowNoIcons=yes
OutputDir=.\installer
OutputBaseFilename=SocialDownloader-Setup
Compression=lzma
SolidCompression=yes
PrivilegesRequired=low

[Files]
Source: "publish\*"; DestDir: "{app}"; Flags: ignoreversion recursesubdirs createallsubdirs
; Upewnij się, że yt-dlp.exe i ffmpeg.exe znajdują się w publish/ (Krok 2)

[Icons]
Name: "{group}\{#MyAppName}"; Filename: "{app}\{#MyAppExe}"

[Run]
Filename: "{app}\{#MyAppExe}"; Description: "Uruchom {#MyAppName}"; Flags: nowait postinstall skipifsilent
```

Kompiluj: `ISCC SocialDownloader.iss` (wiersz polecenia) lub przez kreator Inno Setup Compiler. Efekt: `installer\SocialDownloader-Setup.exe`.

### Opcja B: WiX Toolset

Zainstaluj WiX v3 lub v4, zdefiniuj komponenty z plikami z `publish\`, dodaj skrót w menu Start i regułę uruchomienia `SocialDownloader.exe` po instalacji. WiX daje większą kontrolę, ale wymaga więcej plików XML (`Product.wxs`, `Bundle.wxs` dla bootstrappera).

## Krok 4 — Weryfikacja

1. Zainstaluj wyprodukowany `.exe` na czystej maszynie (lub VM).
2. Uruchom `SocialDownloader.exe`.
3. Uruchom silnik Go na `127.0.0.1:8765` i sprawdź `http://127.0.0.1:8765/healthz`.
4. Wklej link, wybierz jakość i folder, pobierz — komunikaty błędów powinny być przyjazne (brak wyjątków) gdy silnik jest niedostępny.

## Uwagi

- Aplikacja nie posiada własnej ikony (`Resources\app.ico` nie istnieje) — używana jest domyślna ikona Windows. Można dodać plik `.ico` i przywrócić `<ApplicationIcon>` w `SocialDownloader.csproj`.
- `dotnet build SocialDownloader.csproj -c Release` w folderze `windows/` służy do samej weryfikacji kompilacji (wymaga .NET 8 SDK).
- Na maszynie docelowej nie wymagamy uprawnień administratora (`PrivilegesRequired=low`) — wystarczy zapis we wskazanym folderze docelowym.
