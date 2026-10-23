# =============================================================================
# build-windows.ps1 - budowa klienta C#/WPF Social Downloader (Windows)
#
# Uruchomienie: .\build-windows.ps1        (z folderu windows/)
#               .\build-windows.ps1 -SkipBuild -SkipPublish   (tylko walidacja)
# Wymagania: .NET 8 SDK, Windows PowerShell 5.1 lub PowerShell 7+
# =============================================================================

[CmdletBinding()]
param(
    [switch]$SkipBuild,
    [switch]$SkipPublish
)

$ErrorActionPreference = 'Stop'
$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Definition
Set-Location $scriptDir

$csproj    = 'SocialDownloader.csproj'
$publishDir = Join-Path $scriptDir 'publish'
$buildConfig = 'Release'
$runtime   = 'win-x64'

Write-Host '=== Social Downloader - budowa Windows (WPF, .NET 8) ===' -ForegroundColor Cyan
Write-Host 'Folder roboczy: ' $scriptDir -ForegroundColor Gray
Write-Host ''

# ---------------------------------------------------------------------------
# 1. Sprawdzenie czy .NET 8 SDK jest zainstalowane (dotnet --version)
# ---------------------------------------------------------------------------
Write-Host '-> Sprawdzanie .NET 8 SDK ...' -ForegroundColor Yellow

if (!(Get-Command dotnet -ErrorAction SilentlyContinue)) {
    Write-Host 'BLAD: nie znaleziono polecenia dotnet.' -ForegroundColor Red
    Write-Host '      Zainstaluj .NET 8 SDK: https://dotnet.microsoft.com' -ForegroundColor Red
    exit 1
}

$dotnetVersion = dotnet --version 2>&1
if ($LASTEXITCODE -ne 0) {
    Write-Host 'BLAD: nie udalo sie wykonac dotnet --version.' -ForegroundColor Red
    exit 1
}

if ($dotnetVersion -notmatch '^8\.') {
    Write-Host ('BLAD: wymagany .NET 8 SDK, wykryto: ' + $dotnetVersion) -ForegroundColor Red
    exit 1
}

Write-Host '   OK: .NET SDK ' $dotnetVersion -ForegroundColor Green

# ---------------------------------------------------------------------------
# 2. dotnet build SocialDownloader.csproj -c Release
# ---------------------------------------------------------------------------
if (-not $SkipBuild) {
    Write-Host '-> dotnet build ' $csproj ' -c ' $buildConfig ... -ForegroundColor Yellow
    dotnet build $csproj -c $buildConfig
    if ($LASTEXITCODE -ne 0) {
        Write-Host 'BLAD: budowa (' $csproj ') nie powiodla sie.' -ForegroundColor Red
        exit 1
    }
    Write-Host '   OK: budowa zakonczona sukcesem.' -ForegroundColor Green
} else {
    Write-Host '-> Pomijam budowę (-SkipBuild).' -ForegroundColor Gray
}

# ---------------------------------------------------------------------------
# 3. dotnet publish ... -r win-x64 --self-contained false -> publish/
# ---------------------------------------------------------------------------
if (-not $SkipPublish) {
    Write-Host ('-> dotnet publish ' + $csproj + ' -c ' + $buildConfig + ' -r ' + $runtime + ' --self-contained false -o ' + $publishDir + ' ...') -ForegroundColor Yellow
    if (Test-Path $publishDir) { Remove-Item -Recurse -Force $publishDir | Out-Null }
    dotnet publish $csproj -c $buildConfig -r $runtime --self-contained false -o $publishDir
    if ($LASTEXITCODE -ne 0) {
        Write-Host 'BLAD: publikacja nie powiodla sie.' -ForegroundColor Red
        exit 1
    }
    Write-Host '   OK: opublikowano do ' $publishDir -ForegroundColor Green
} else {
    Write-Host '-> Pomijam publikację (-SkipPublish).' -ForegroundColor Gray
}

# ---------------------------------------------------------------------------
# 4. yt-dlp.exe i ffmpeg.exe - skrypt ich nie kopiuje (instrukcja w komentarzach)
# ---------------------------------------------------------------------------
Write-Host ''
Write-Host '>> KROK RECZNY: dolacz yt-dlp.exe i ffmpeg.exe do folderu publish/' -ForegroundColor Yellow
Write-Host '   - yt-dlp.exe : https://github.com/yt-dlp/yt-dlp/releases' -ForegroundColor Gray
Write-Host '   - ffmpeg.exe : https://ffmpeg.org/download.html (build essentials dla Windows)' -ForegroundColor Gray
Write-Host '   Skopiuj oba pliki do: ' $publishDir -ForegroundColor Gray
Write-Host '   Klient WPF wywoluje je bezposrednio z katalogu aplikacji (lub z podfolderu tools).' -ForegroundColor Gray

# ---------------------------------------------------------------------------
# 5. Instalator Inno Setup lub WiX - tylko instrukcja, nie tworzy instalatora
# ---------------------------------------------------------------------------
Write-Host ''
Write-Host '>> KROK RECZNY: zbuduj instalator (Inno Setup lub WiX) - skrypt nie tworzy instalatora.' -ForegroundColor Yellow
Write-Host '   Inno Setup: https://jrsoftware.org/is.php' -ForegroundColor Gray
Write-Host '   - napisz skrypt .iss (np. SocialDownloader.iss) wskazujacy na ' $publishDir -ForegroundColor Gray
Write-Host '   - skompiluj: ISCC SocialDownloader.iss -> installer\SocialDownloader-Setup.exe' -ForegroundColor Gray
Write-Host '   WiX: https://wixtoolset.org/ - zdefiniuj komponenty z plikow publish/, skrót w menu Start.' -ForegroundColor Gray
Write-Host '   Szczegoly: patrz BUILD_WINDOWS.md.' -ForegroundColor Gray

# ---------------------------------------------------------------------------
# Podsumowanie co zrobil skrypt
# ---------------------------------------------------------------------------
Write-Host ''
Write-Host '============================================================' -ForegroundColor Cyan
Write-Host 'PODSUMOWANIE' -ForegroundColor Cyan
Write-Host '============================================================' -ForegroundColor Cyan
Write-Host ('  Sprawdzono .NET 8 SDK : ' + $dotnetVersion)
if (-not $SkipBuild) {
    Write-Host '  dotnet build          : OK  (' $csproj ' -c ' $buildConfig ')' -ForegroundColor Green
} else {
    Write-Host '  dotnet build          : pominiete' -ForegroundColor Gray
}
if (-not $SkipPublish) {
    Write-Host '  dotnet publish        : OK  (-r ' $runtime ' --self-contained false -> ' $publishDir ')' -ForegroundColor Green
} else {
    Write-Host '  dotnet publish        : pominiete' -ForegroundColor Gray
}
Write-Host '  yt-dlp.exe / ffmpeg   : do dodania recznie do ' $publishDir ' (patrz komentarze powyzsze)'
Write-Host '  Instalator            : nie utworzono - patrz BUILD_WINDOWS.md (Inno Setup / WiX)'
Write-Host ''
Write-Host 'Koniec skryptu build-windows.ps1.' -ForegroundColor Cyan
