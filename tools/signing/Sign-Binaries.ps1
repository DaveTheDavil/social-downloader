# =============================================================================
# Sign-Binaries.ps1 - podpisywanie binarek SocialDownloader (Windows code signing)
# =============================================================================
# Wspolny pipeline dla kazdej trasy certyfikatu:
#   1) plik .pfx            (SD_SIGN_PFX + SD_SIGN_PFX_PASSWORD),
#   2) cert w user-store    (SD_SIGN_SUBJECT albo SD_SIGN_THUMBPRINT; trasa
#                           Certum SimplySign - klucz w chmurze, widoczny
#                           jako cert w Cert:\CurrentUser\My),
#   3) brak konfiguracji    = STUB: pliki NIEPODPISANE, ostrzezenie
#                           "UNSIGNED (stub)", exit 0 - pakowanie trwa dalej.
#
# Konfiguracja TYLKO przez zmienne srodowiskowe (sekrety nigdy do repo/CLI):
#   SD_SIGN_PFX            - sciezka pliku .pfx
#   SD_SIGN_PFX_PASSWORD   - haslo do .pfx (SEKRET; puste = pfx bez hasla)
#   SD_SIGN_SUBJECT        - fragment Subject/CN certu w store
#   SD_SIGN_THUMBPRINT     - thumbprint SHA1 (dokladniejszy niz /n)
#   SD_SIGN_TIMESTAMP_URL  - serwer RFC3161 (default: http://timestamp.digicert.com)
# Priorytet: PFX > THUMBPRINT > SUBJECT.
#
# Co podpisujemy (default -Targets):
#   windows\publish\SocialDownloader.exe   (signtool)
#   engine\downloader.exe                  (signtool)
#   setup\instalator.ps1                   (Set-AuthenticodeSignature)
#
# Zasady twarde:
#   - timestamp RFC3161 ZAWSZE (/fd SHA256 /td SHA256 /tr ...) - podpis
#     przezywa wygasniecie certu; od 2026-02-27 cert CS max 459 dni (reissue),
#   - plik w uzyciu (np. dev engine odpalony z engine\) -> wykrycie procesu
#     i SKIP z ostrzezeniem; NIE zabijamy procesow,
#   - fail-soft: brak konfiguracji -> exit 0 + warn,
#   - nieudane podpisanie -> exit 2 (pakowanie przerywa; pol podpisana
#     paczka nie wychodzi), flaga -Strict: brak configu tez = exit 2,
#   - -Verify: weryfikacja podpisow (signtool verify /pa /all + .ps1) -
#     klikalne takze BEZ konfiguracji (stub-verify dla surowych plikow).
#
# Uzycie:
#   powershell -ExecutionPolicy Bypass -File tools\signing\Sign-Binaries.ps1
#              [-Verify] [-Strict] [-SigntoolPath <sciezka>] [-Targets a,b,c]
# Exit: 0 = OK albo stub; 2 = nieudane podpisanie; 3 = blad srodowiska.
# =============================================================================

[CmdletBinding()]
param(
    [string[]]$Targets = @(),
    [string]$SigntoolPath = '',
    [switch]$Verify,
    [switch]$Strict
)

$ErrorActionPreference = 'Stop'

$repoRoot = Split-Path -Parent (Split-Path -Parent $PSScriptRoot)

# --- konfiguracja z srodowiska ---
$pfxPath = [Environment]::GetEnvironmentVariable('SD_SIGN_PFX')
$pfxPass = [Environment]::GetEnvironmentVariable('SD_SIGN_PFX_PASSWORD')
$subject = [Environment]::GetEnvironmentVariable('SD_SIGN_SUBJECT')
$thumb   = [Environment]::GetEnvironmentVariable('SD_SIGN_THUMBPRINT')
$tsUrl   = [Environment]::GetEnvironmentVariable('SD_SIGN_TIMESTAMP_URL')
if ([string]::IsNullOrWhiteSpace($tsUrl)) { $tsUrl = 'http://timestamp.digicert.com' }
if ($pfxPath) { $pfxPath = $pfxPath.Trim('"') }
if ($thumb)   { $thumb = ($thumb -replace '\s', '').ToUpperInvariant() }

# --- helpery ---

function Test-FileLocked {
    param([string]$Path)
    try {
        $fs = [IO.File]::Open($Path, [IO.FileMode]::Open, [IO.FileAccess]::ReadWrite, [IO.FileShare]::None)
        $fs.Dispose()
        return $false
    } catch {
        return $true
    }
}

function Get-LockedByProcesses {
    param([string]$Path)
    $full = $null
    try { $full = (Get-Item -LiteralPath $Path -ErrorAction Stop).FullName } catch { return @() }
    return @(Get-Process -ErrorAction SilentlyContinue |
        Where-Object { $_.Path -and $_.Path -eq $full } |
        Select-Object Id, ProcessName)
}

function Test-CodeSigningEku {
    param($Cert)
    $eku = $null
    foreach ($e in $Cert.Extensions) {
        if ($e -is [System.Security.Cryptography.X509Certificates.X509EnhancedKeyUsageExtension]) { $eku = $e; break }
    }
    if (-not $eku) { return $true }   # brak EKU = cert dopuszczalny do wszystkiego
    return [bool]($eku.EnhancedKeyUsages | Where-Object { $_.Value -eq '1.3.6.1.5.5.7.3.3' })
}

function Get-StoreCert {
    # szuka w CurrentUser\My i LocalMachine\My; preferuje cert z EKU Code Signing,
    # nie wygasly, o najpozniejszym NotAfter
    param([string]$Thumb, [string]$Subject)
    $best = $null
    foreach ($loc in @('CurrentUser', 'LocalMachine')) {
        $store = New-Object System.Security.Cryptography.X509Certificates.X509Store('My', $loc)
        try { $store.Open([System.Security.Cryptography.X509Certificates.OpenFlags]::ReadOnly) } catch { continue }
        try {
            $sel = @()
            if ($Thumb) {
                $sel = @($store.Certificates | Where-Object { $_.Thumbprint -eq $Thumb })
            } else {
                $sel = @($store.Certificates | Where-Object { $Subject -and $_.Subject -like "*$Subject*" })
            }
            $now = Get-Date
            $sel = @($sel | Where-Object { $_.NotAfter -gt $now } | Sort-Object -Property NotAfter -Descending)
            foreach ($c in $sel) {
                if (Test-CodeSigningEku $c) { return $c }
                if (-not $best) { $best = $c }
            }
        } finally { $store.Close() }
    }
    return $best
}

function Resolve-Signtool {
    param([string]$Explicit)
    if ($Explicit) {
        if (Test-Path -LiteralPath $Explicit) { return $Explicit }
        Write-Host "BLAD: -SigntoolPath nie istnieje: $Explicit" -ForegroundColor Red
        return $null
    }
    $kitBin = Join-Path ${env:ProgramFiles(x86)} 'Windows Kits\10\bin'
    $cands = @()
    try {
        $cands = @(Get-ChildItem -LiteralPath $kitBin -Recurse -Filter 'signtool.exe' -File -ErrorAction SilentlyContinue |
            Where-Object { $_.DirectoryName -like '*\x64' } |
            Sort-Object -Property { try { [version]$_.Directory.Name } catch { [version]'0.0' } } -Descending)
    } catch { }
    if ($cands.Count -gt 0) { return $cands[0].FullName }
    $cmd = Get-Command 'signtool.exe' -ErrorAction SilentlyContinue
    if ($cmd) { return $cmd.Source }
    return $null
}

# --- targets ---
if ($Targets.Count -eq 0) {
    $Targets = @(
        'windows\publish\SocialDownloader.exe',
        'engine\downloader.exe',
        'setup\instalator.ps1'
    )
}

$exeTargets = New-Object System.Collections.Generic.List[string]
$ps1Targets = New-Object System.Collections.Generic.List[string]
$missing    = New-Object System.Collections.Generic.List[string]
foreach ($t in $Targets) {
    $p = Join-Path $repoRoot $t
    if (-not (Test-Path -LiteralPath $p)) { $missing.Add($t); continue }
    if ([IO.Path]::GetExtension($p).ToLowerInvariant() -eq '.ps1') { $ps1Targets.Add($p) }
    else { $exeTargets.Add($p) }
}

$signed  = New-Object System.Collections.Generic.List[string]
$skipped = New-Object System.Collections.Generic.List[string]
$failed  = New-Object System.Collections.Generic.List[string]

$stubMode = (-not $pfxPath -and -not $thumb -and -not $subject)

# --- stub bez -Verify: czysty przebieg testowy, nic nie podpisujemy ---
if ($stubMode -and -not $Verify) {
    Write-Host ''
    Write-Host 'PODPIS: UNSIGNED (stub) - brak certyfikatu w srodowisku (SD_SIGN_*).' -ForegroundColor DarkYellow
    Write-Host '  Trasa PFX  : SD_SIGN_PFX + SD_SIGN_PFX_PASSWORD'
    Write-Host '  Trasa store: SD_SIGN_SUBJECT lub SD_SIGN_THUMBPRINT (Certum SimplySign)'
    Write-Host '  Instrukcja: tools\signing\README-SIGNING.md'
    if ($missing.Count -gt 0) {
        Write-Host ("  brakujace targety (pomijam): " + ($missing -join ', ')) -ForegroundColor DarkYellow
    }
    if ($Strict) { Write-Host '  -Strict: brak konfiguracji = blad.' -ForegroundColor Red; exit 2 }
    exit 0
}

# --- konfiguracja certu (tylko nie-stub) ---
$cert = $null
$ident = 'stub-verify (bez podpisywania)'
if (-not $stubMode) {
    if ($pfxPath) {
        if (-not (Test-Path -LiteralPath $pfxPath)) {
            Write-Host "BLAD: SD_SIGN_PFX wskazuje na nieistniejacy plik: $pfxPath" -ForegroundColor Red
            exit 3
        }
        try {
            $certArgs = @($pfxPath, $pfxPass)
            $cert = New-Object System.Security.Cryptography.X509Certificates.X509Certificate2 -ArgumentList $certArgs
        } catch {
            Write-Host "BLAD: nie mozna otworzyc PFX ($($_.Exception.Message))" -ForegroundColor Red
            exit 3
        }
        $ident = $cert.Subject
    } else {
        $cert = Get-StoreCert -Thumb $thumb -Subject $subject
        if (-not $cert) {
            Write-Host 'BLAD: certyfikatu nie ma w store (CurrentUser/LocalMachine My).' -ForegroundColor Red
            Write-Host '  Trasa SimplySign: odpal SimplySign Desktop i zaloguj sie do klucza w chmurze,' -ForegroundColor Red
            Write-Host '  po czym sprawdz: Get-ChildItem Cert:\CurrentUser\My -CodeSigningCert' -ForegroundColor Red
            exit 3
        }
        $ident = $cert.Subject
    }
    Write-Host ''
    Write-Host "Podpisuje jako: $ident" -ForegroundColor Cyan
    Write-Host "Timestamp RFC3161: $tsUrl"
}

$signtool = Resolve-Signtool -Explicit $SigntoolPath
if (-not $signtool) {
    Write-Host 'BLAD: signtool.exe nie znaleziony (Windows Kits 10 - bin\x64 albo PATH).' -ForegroundColor Red
    exit 3
}
Write-Host "signtool: $signtool"

# --- podpis .exe/.dll przez signtool (JELI config jest) ---
if (-not $stubMode) {
    $signArgs = @('sign', '/fd', 'SHA256', '/td', 'SHA256', '/tr', $tsUrl)
    if ($pfxPath) {
        $signArgs += @('/f', $pfxPath)
        if (-not [string]::IsNullOrEmpty($pfxPass)) { $signArgs += @('/p', $pfxPass) }
    } elseif ($thumb) {
        $signArgs += @('/sha1', $thumb)
    } else {
        $signArgs += @('/n', $subject)
    }

    foreach ($p in $exeTargets) {
        $rel = $p.Substring($repoRoot.Length).TrimStart('\')
        if (Test-FileLocked $p) {
            $procs = Get-LockedByProcesses $p
            $who = 'proces nieustalony'
            if ($procs.Count -gt 0) {
                $who = (@($procs | ForEach-Object { "$($_.ProcessName)(PID $($_.Id))" }) -join ', ')
            }
            Write-Host "  SKIP (plik w uzyciu): $rel - zajmuje: $who; zamknij i przelicz ponizszy etap." -ForegroundColor DarkYellow
            $skipped.Add($rel)
            continue
        }
        & $signtool @signArgs $p
        if ($LASTEXITCODE -ne 0) {
            Write-Host "  BLAD podpisu: $rel (signtool exit $LASTEXITCODE)" -ForegroundColor Red
            $failed.Add($rel)
        } else {
            Write-Host "  OK podpisano: $rel" -ForegroundColor Green
            $signed.Add($rel)
        }
    }

    # --- .ps1 przez Set-AuthenticodeSignature (ten sam cert) ---
    foreach ($p in $ps1Targets) {
        $rel = $p.Substring($repoRoot.Length).TrimStart('\')
        if (Test-FileLocked $p) {
            Write-Host "  SKIP (plik w uzyciu): $rel" -ForegroundColor DarkYellow
            $skipped.Add($rel)
            continue
        }
        try {
            $r = Set-AuthenticodeSignature -FilePath $p -SigningCert $cert -TimestampServer $tsUrl -HashAlgorithm SHA256
            if ($r.Status -eq 'Valid') {
                Write-Host "  OK podpisano: $rel" -ForegroundColor Green
                $signed.Add($rel)
            } else {
                Write-Host "  BLAD podpisu: $rel (status $($r.Status): $($r.StatusMessage))" -ForegroundColor Red
                $failed.Add($rel)
            }
        } catch {
            Write-Host "  BLAD podpisu: $rel ($($_.Exception.Message))" -ForegroundColor Red
            $failed.Add($rel)
        }
    }
}

# --- weryfikacja (-Verify; takze w trybie stub-verify) ---
if ($Verify) {
    Write-Host '-> Weryfikacja podpisow ...' -ForegroundColor Yellow
    foreach ($p in $exeTargets) {
        $rel = $p.Substring($repoRoot.Length).TrimStart('\')
        & $signtool verify /pa /all $p
        if ($LASTEXITCODE -ne 0) {
            Write-Host "  UWAGA: weryfikacja nie przeszla: $rel" -ForegroundColor DarkYellow
        } else {
            Write-Host "  verify OK: $rel" -ForegroundColor Green
        }
    }
    foreach ($p in $ps1Targets) {
        $rel = $p.Substring($repoRoot.Length).TrimStart('\')
        $s = Get-AuthenticodeSignature -FilePath $p
        if ($s.Status -eq 'Valid') {
            Write-Host "  verify OK: $rel (podpisano jako $($s.SignerCertificate.Subject))" -ForegroundColor Green
        } else {
            Write-Host "  UWAGA: $rel - status podpisu: $($s.Status)" -ForegroundColor DarkYellow
        }
    }
}

# --- podsumowanie ---
if ($missing.Count -gt 0) {
    Write-Host ("Brakujace targety: " + ($missing -join ', ')) -ForegroundColor DarkYellow
}
Write-Host ''
Write-Host ("PODSUMOWANIE: podpisano $($signed.Count), skip $($skipped.Count), bledy $($failed.Count)") -ForegroundColor Cyan
if ($failed.Count -gt 0) {
    exit 2
}
exit 0
