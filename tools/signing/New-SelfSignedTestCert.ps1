# =============================================================================
# New-SelfSignedTestCert.ps1 - DEV-ONLY: self-signed cert do TESTOW podpisu
# =============================================================================
# UWAGA (decyzja 2026-10-24): helper zostaje w repo, ale celowo NIE jest
# uruchamiany. Self-signed NIE daje zaufanego wydawcy - SmartScreen u
# uzytkownikow nadal ostrzega ("unknown publisher"). Wartosc = test
# calego pipeline podpisu (signtool/timestamp/verify) bez wydawania pieniedzy.
# Komercyjna trasa: Certum Standard Code Signing in the Cloud - patrz
# tools\signing\README-SIGNING.md.
#
# Wygenerowany cert to .pfx w %TEMP%\sd-dev-sign (POZA repo!) - sekrety
# nie wchodza do gita. Certyfikat jest usuwany z user-store po eksporcie
# (zostaje tylko plik PFX, ktorym steruje sie zmiennymi SD_SIGN_*).
#
# Uzycie (uruchomic dopiero przy decyzji o DIY pomiarze pipeline):
#   powershell -ExecutionPolicy Bypass -File tools\signing\New-SelfSignedTestCert.ps1
#   # potem:
#   $env:SD_SIGN_PFX          = "$env:TEMP\sd-dev-sign\LocalDev.pfx"
#   $env:SD_SIGN_PFX_PASSWORD = '<haslo z konsoli>'
#   powershell -ExecutionPolicy Bypass -File tools\signing\Sign-Binaries.ps1 -Verify
# =============================================================================

[CmdletBinding()]
param(
    [string]$PfxOut = '',
    [string]$Password = '',
    [string]$CommonName = 'SocialDownloader DEV',
    [int]$Years = 1
)

$ErrorActionPreference = 'Stop'

if (-not $PfxOut) { $PfxOut = Join-Path $env:TEMP 'sd-dev-sign\LocalDev.pfx' }
if (-not $Password) {
    $Password = [Environment]::GetEnvironmentVariable('SD_SIGN_PFX_PASSWORD')
}
if (-not $Password) {
    # haslo losowe, pokazane raz na konsoli (dev-only, plik lezy w TEMP)
    $Password = 'dev-' + [IO.Path]::GetRandomFileName() -replace '\.', ''
}

New-Item -ItemType Directory -Force -Path (Split-Path -Parent $PfxOut) | Out-Null
if (Test-Path $PfxOut) { Remove-Item $PfxOut -Force }

Write-Host '-> Tworze self-signed CodeSigning cert (CurrentUser\My) ...' -ForegroundColor Yellow
$cert = New-SelfSignedCertificate `
    -Type CodeSigningCert `
    -Subject "CN=$CommonName" `
    -KeyUsage DigitalSignature `
    -HashAlgorithm SHA256 `
    -KeyExportPolicy Exportable `
    -CertStoreLocation Cert:\CurrentUser\My `
    -NotAfter (Get-Date).AddYears($Years)

Write-Host '-> Export do PFX ...' -ForegroundColor Yellow
$sec = ConvertTo-SecureString -String $Password -AsPlainText -Force
Export-PfxCertificate -Cert $cert -FilePath $PfxOut -Password $sec | Out-Null

try {
    Remove-Item -LiteralPath "Cert:\CurrentUser\My\$($cert.Thumbprint)" -Force
} catch {
    Write-Host "  UWAGA: nie udalo sie usunac certu ze store: $($_.Exception.Message)" -ForegroundColor DarkYellow
}

Write-Host ''
Write-Host 'Self-signed DEV cert gotowy:' -ForegroundColor Green
Write-Host "  PFX        : $PfxOut"
Write-Host "  Subject    : CN=$CommonName"
Write-Host "  Thumbprint : $($cert.Thumbprint)"
Write-Host "  Wazny do   : $($cert.NotAfter)"
Write-Host "  Haslo      : $Password   (SEKRET - pokazane tylko tu, nie zapisuje do pliku)"
Write-Host ''
Write-Host 'Test pipeline:' -ForegroundColor Cyan
Write-Host ("  `$env:SD_SIGN_PFX          = '{0}'" -f $PfxOut)
Write-Host ("  `$env:SD_SIGN_PFX_PASSWORD = '{0}'" -f $Password)
Write-Host '  powershell -ExecutionPolicy Bypass -File tools\signing\Sign-Binaries.ps1 -Verify'
