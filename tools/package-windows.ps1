# =============================================================================
# package-windows.ps1 - sklada przenosny pakiet instalacyjny Windows
#                       (SocialDownloader-<wersja>-win64.zip)
#
# UWAGA: skrypt NIC nie kompiluje — zaklada, ze binarki sa juz zbudowane:
#   1) silnik:   w engine/   -> go build -o downloader.exe .
#              (obok musza lezec yt-dlp.exe, ffmpeg.exe, public-key.pem,
#               ads-config.json — pliki pobierane/aktualizowane przez silnik)
#   2) klient:   w windows/  ->
#        dotnet publish SocialDownloader.csproj -c Release -r win-x64
#            --self-contained true -p:DebugType=none -p:DebugSymbols=false
#            -o publish
#      (self-contained = wbudowane .NET 8, nic nie trzeba instalowac;
#       --self-contained false NIE jest akceptowane — pakiet musialby
#       dolaczac instalator windowsdesktop-runtime i zabiegac o niego
#       przed pierwszym uruchomieniem)
#
# SEKRETY (license.key, ig-cookie.txt, *.pem oprocz publicznego, keys/) sa
# celowo NIE kopiowane; po zlozeniu pakietu skrypt skanuje drzewo stagingu
# i przerywa z bledem przy czegokolwiek podejrzanym.
#
# Uruchomienie: powershell -File tools\package-windows.ps1 [-Version 1.0.0]
# Wynik:        <Desktop>\INSTALKA\SocialDownloader-<wersja>-win64.zip
# =============================================================================

[CmdletBinding()]
param(
    [string]$Version = '1.0.0',
    # Folder z wynikiem dotnet publish klienta (self-contained!); pusty -> windows\publish
    [string]$AppDir = ''
)

$ErrorActionPreference = 'Stop'
Add-Type -AssemblyName System.IO.Compression.FileSystem | Out-Null

$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
$engineDir = Join-Path $repoRoot 'engine'
if ([string]::IsNullOrWhiteSpace($AppDir)) {
    $AppDir = Join-Path $PSScriptRoot 'windows\publish'
    if (-not (Test-Path $AppDir)) { $AppDir = Join-Path (Join-Path $repoRoot 'windows') 'publish' }
}

$desktop = [Environment]::GetFolderPath('Desktop')   # Desktop moze byc na OneDrive
$instalkaDir = Join-Path $desktop 'INSTALKA'
$staging = Join-Path $instalkaDir 'SocialDownloader'
$zipPath = Join-Path $instalkaDir ("SocialDownloader-$Version-win64.zip")

$allowedEngineExe = @('downloader.exe', 'yt-dlp.exe', 'ffmpeg.exe', 'ffprobe.exe') # optional ffprobe: if a build uses it

Write-Host '=== Social Downloader - pakowanie Windows ===' -ForegroundColor Cyan

# ---------------------------------------------------------------------------
# 1. Walidacja wejscia (binarki musza istniec zanim cokolwiek zlozymy)
# ---------------------------------------------------------------------------
Write-Host '-> Walidacja wejscia ...' -ForegroundColor Yellow

$engineFiles = @('downloader.exe', 'yt-dlp.exe', 'ffmpeg.exe', 'public-key.pem', 'ads-config.json')
foreach ($f in $engineFiles) {
    if (-not (Test-Path (Join-Path $engineDir $f))) {
        Write-Host "BLAD: brak pliku silnika: $engineDir\$f" -ForegroundColor Red
        if ($f -eq 'ffmpeg.exe') { Write-Host '   ffmpeg.exe dokpiec obok downloader.exe (PATH tez zadziala, ale pakiet ma byc samowystarczalny).' -ForegroundColor Red }
        if ($f -eq 'yt-dlp.exe') { Write-Host '   yt-dlp.exe pobiera silnik automatycznie przy starcie, ale pakiet ma byc samowystarczalny.' -ForegroundColor Red }
        exit 1
    }
}
Write-Host "   engine: OK ($($engineFiles -join ', '))"

$appExe = Join-Path $AppDir 'SocialDownloader.exe'
if (-not (Test-Path $appExe)) {
    Write-Host "BLAD: brak $appExe" -ForegroundColor Red
    Write-Host '   Zbuduj klienta: dotnet publish SocialDownloader.csproj -c Release -r win-x64 --self-contained true -o publish  (w windows/)' -ForegroundColor Red
    exit 1
}
if (-not (Test-Path (Join-Path $AppDir 'hostfxr.dll'))) {
    Write-Host "BLAD: $AppDir NIE jest publishem self-contained (brak hostfxr.dll)." -ForegroundColor Red
    Write-Host '   Powtorz publish z --self-contained true, albo zmien strategie na framework+instalator runtime' -ForegroundColor Red
    Write-Host '   (https://aka.ms/dotnet/8.0/windowsdesktop-runtime-win-x64.exe + instrukcja w README pakietu).' -ForegroundColor Red
    exit 1
}
Write-Host '   app: OK (self-contained, hostfxr.dll obecny)'

# ---------------------------------------------------------------------------
# 2. Czysty staging <Desktop>\INSTALKA\SocialDownloader
# ---------------------------------------------------------------------------
Write-Host "-> Staging: $staging ..." -ForegroundColor Yellow
if (Test-Path $staging) { Remove-Item -Recurse -Force $staging }
New-Item -ItemType Directory -Force -Path $staging | Out-Null
New-Item -ItemType Directory -Force -Path (Join-Path $staging 'engine') | Out-Null
New-Item -ItemType Directory -Force -Path (Join-Path $staging 'app') | Out-Null

# engine: tylko dozwolony zestaw (license.key/ig-cookie.txt celowo POMINIETE)
foreach ($f in $engineFiles) { Copy-Item (Join-Path $engineDir $f) -Destination (Join-Path $staging "engine\$f") }
foreach ($extra in @('ffprobe.exe')) { # opcjonalny, gdyby stal w engine/ przy przyszlym buildzie
    if (Test-Path (Join-Path $engineDir $extra)) { Copy-Item (Join-Path $engineDir $extra) -Destination (Join-Path $staging "engine\$extra") }
}

# app: caly folder publish (flat, obiekty obj/ bin/ nie wchodza; print tylko lacznie)
Copy-Item -Path (Join-Path $AppDir '*') -Destination (Join-Path $staging 'app') -Recurse -Force

# VERSION.txt (git short hash + data pakowania)
$shortHash = (& git -C $repoRoot rev-parse --short HEAD) 2>$null
if ($LASTEXITCODE -ne 0) { $shortHash = 'brak-gita' }
$stamp = Get-Date -Format 'yyyy-MM-dd HH:mm'
[IO.File]::WriteAllText((Join-Path $staging 'VERSION.txt'),
    "SocialDownloader $Version`r`nCommit: $shortHash`r`nBuild date: $stamp`r`n",
    (New-Object System.Text.UTF8Encoding($false)))

# START.bat (ASCII - cmd i tak czyta ANSI/OEM)
$bat = @'
@echo off
title Social Downloader - start
rem Uruchamia silnik (ukryte okno, log w %TEMP%\sd-engine.log) i klienta GUI.
powershell -NoProfile -Command "Start-Process -FilePath '%~dp0engine\downloader.exe' -WorkingDirectory '%~dp0engine' -ArgumentList '-addr','127.0.0.1:8765' -WindowStyle Hidden -RedirectStandardOutput '%TEMP%\sd-engine.log' -RedirectStandardError '%TEMP%\sd-engine.err.log'"
start "" "%~dp0app\SocialDownloader.exe"
'@
$bat = ($bat -replace "`r`n", "`n") -replace "`n", "`r`n"
[IO.File]::WriteAllText((Join-Path $staging 'START.bat'), ($bat))
Write-Host '   engine/app/VERSION/START. OK' -ForegroundColor Green

# ---------------------------------------------------------------------------
# 3. README.txt (UTF-8 z BOM - bezpieczny dla Notatnika w Win10)
# ---------------------------------------------------------------------------
$readme = @"
Social Downloader - pakiet przenosny (Windows 10/11 x64)
=========================================================

WYMAGANIA
- Windows 10 lub 11 (64-bit) i Internet.
- Niczego nie instaluj - runtime .NET 8 jest wbudowany w folder app\.

URUCHOMIENIE
1. Rozpakuj CALY folder zip (nie uruchamiaj pojedynczych plikow spod zipa).
2. Kliknij dwukrotnie START.bat - silnik wystartuje bez okna,
   okno programu pojawi sie od razu.
   Recznie (zamiast START.bat): uruchom engine\downloader.exe,
   potem app\SocialDownloader.exe.
3. Pierwsze uruchomienie: SmartScreen/antywirus moze zapytac o pliki
   bez cyfrowego podpisu. Wybierz "Wiecej informacji", potem
   "Uruchom mimo wszystko" (dotyczy zwykle yt-dlp.exe/downloader.exe).

JAK TO DZIALA
- Program wysyla zadania do lokalnego silnika (engine\downloader.exe),
  ktory nasluchuje wylacznie na 127.0.0.1:8765 - nie wychodzi poza
  komputer.
- Obslugiwane platformy: Facebook, Instagram, TikTok, YouTube
  (limit 10 minut; prywatne tresci YouTube zablokowane).

TRYB DARMOWY / LICENCJA
- Na nowym komputerze program wystartuje w trybie darmowym (reklamy,
  HD do 720p).
- Licencja premium jest zwiazana z komputerem (Hardware ID) i nie
  jest dziedziczona z innego PC.

ATEST/ODPOWIEDZNOSC
- Aplikacja sluzy do pobierania wylacznie tresci, na ktore masz prawa
  lub ktore sa w domenie publicznej. Uzytkownik ponosi pelna
  odpowiedzialnosc za przestrzeganie regulaminow platform
  i praw autorskich.

SESJA INSTAGRAM (opcjonalnie)
- Tresci prywatne/z konta wymagaja zalogowania: w kliencie uzyj
  przycisku "Zapisz sesje" (albo recznie utworz engine\ig-cookie.txt
  z ciastkiem sesji). Bez tego dzialaja tylko tresci publiczne.
"@
[IO.File]::WriteAllText((Join-Path $staging 'README.txt'), $readme, (New-Object System.Text.UTF8Encoding($true)))
Write-Host '   README.txt OK' -ForegroundColor Green

# ---------------------------------------------------------------------------
# 4. TWARDY SEKRET-GATE - skan stagingu, FAIL przy czemkolwiek podejrzanym
# ---------------------------------------------------------------------------
Write-Host '-> Sekret-gate ...' -ForegroundColor Yellow
$vio = New-Object System.Collections.Generic.List[string]
$allowedExe = ($allowedEngineExe + 'SocialDownloader.exe' + 'createdump.exe')
Get-ChildItem -Path $staging -Recurse -Force | ForEach-Object {
    $rel = $_.FullName.Substring($staging.Length).TrimStart('\')
    $name = $_.Name
    $underApp = $rel -like 'app\*'
    if ($name -eq 'license.key') { $vio.Add($rel) }
    elseif ($name -like 'ig-cookie*') { $vio.Add($rel) }
    elseif ($name -like 'private*') { $vio.Add($rel) }
    elseif ($_.Extension -eq '.pem' -and $name -ne 'public-key.pem') { $vio.Add($rel) }
    elseif ($_.PSIsContainer -and ($name -eq 'keys' -or $name -eq 'obj')) { $vio.Add($rel + '\') }
    elseif ($name -like '.git*') { $vio.Add($rel) }
    elseif ($name -eq 'SESSION_NOTES.md') { $vio.Add($rel) }
    elseif ($_.Extension -in '.go', '.cs', '.swift', '.xaml') { $vio.Add($rel) }
    elseif ($_.Extension -eq '.pdb' -and -not $underApp) { $vio.Add($rel) }
    elseif (-not $_.PSIsContainer -and $_.Extension -eq '.exe' -and -not $underApp) {
        if ($allowedExe -notcontains $name) { $vio.Add($rel) }
    }
}
if ($vio.Count -gt 0) {
    Write-Host 'BLAD: sekret-gate ZABLOKOWAL pakiet:' -ForegroundColor Red
    $vio | ForEach-Object { Write-Host "   $_" -ForegroundColor Red }
    Write-Host 'Pakiet NIE zostal zipowany. Usun naruszenia z stagingu i po prostu odpal skrypt ponownie.' -ForegroundColor Red
    exit 1
}
Write-Host "   PASS ($((Get-ChildItem $staging -Recurse -File).Count) plikow czystych)" -ForegroundColor Green

# ---------------------------------------------------------------------------
# 5. ZIP + README-INSTALKA obok + hash/rozmiar/top-level lista
# ---------------------------------------------------------------------------
if (Test-Path $zipPath) { Remove-Item -Force $zipPath }
Write-Host "-> Zip: $zipPath ..." -ForegroundColor Yellow
[IO.Compression.ZipFile]::CreateFromDirectory($staging, $zipPath, [IO.Compression.CompressionLevel]::Optimal, $false)

Copy-Item (Join-Path $staging 'README.txt') -Destination (Join-Path $instalkaDir 'README-INSTALKA.txt') -Force

$zipSize = (Get-Item $zipPath).Length
$sha = (Get-FileHash $zipPath -Algorithm SHA256).Hash
Write-Host ''
Write-Host '=== GOTOWE ===' -ForegroundColor Cyan
Write-Host ("ZIP    : $zipPath")
Write-Host ('Rozmiar: {0:N1} MB' -f ($zipSize / 1MB))
Write-Host ("SHA256 : $sha")
Write-Host 'TOP-LEVEL pakietu:'
Get-ChildItem $staging | ForEach-Object {
    if ($_.PSIsContainer) {
        $s = (Get-ChildItem $_.FullName -Recurse -File | Measure-Object Length -Sum).Sum
        Write-Host ('  {0}/  ({1} plikow, {2:N1} MB)' -f $_.Name, (Get-ChildItem $_.FullName -Recurse -File).Count, ($s / 1MB))
    } else {
        Write-Host ('  {0}  ({1:N0} B)' -f $_.Name, $_.Length)
    }
}
