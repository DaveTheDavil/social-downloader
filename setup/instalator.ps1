# =============================================================================
# instalator.ps1 — kreator instalacji Social Downloader (WPF GUI, per-user)
#
# Pozycja w pakiecie: installer\instalator.ps1 (obok INSTALATOR.bat w root).
# Robi:  engine\ + app\ -> folder docelowy (default %LOCALAPPDATA%\Programs\...),
#        SocialDownloader-Start.vbs (cichy launcher START.bat bez konsoli),
#        skróty Menu Start + Pulpit (ikona z exe klienta),
#        wpis HKCU w "Programy i funkcje" + Odinstaluj.bat/Odinstaluj.ps1,
#        opcjonalna reguła firewall (tryb LAN; fail-soft bez admina),
#        README-INSTALACJA.txt.
#
# Tryby:  GUI (default) | -Silent [-Target "..."] [-WithFirewall] | -SelfTest
# =============================================================================
[CmdletBinding()]
param(
    [string]$Target = '',
    [switch]$Silent,
    [switch]$SelfTest,
    [switch]$WithFirewall
)
$ErrorActionPreference = 'Stop'
Add-Type -AssemblyName PresentationFramework
Add-Type -AssemblyName PresentationCore
Add-Type -AssemblyName WindowsBase
Add-Type -AssemblyName System.Windows.Forms

$stagingRoot = Split-Path -Parent $PSScriptRoot
$engineSrc   = Join-Path $stagingRoot 'engine'
$appSrc      = Join-Path $stagingRoot 'app'
$logoSrc     = Join-Path $PSScriptRoot 'dtd-logo.png'

$AppVersion = '1.0.0'
$vtx = Join-Path $stagingRoot 'VERSION.txt'
if (Test-Path $vtx) {
    $first = (Get-Content $vtx -TotalCount 1 | Select-Object -First 1)
    if ($first -match '([0-9]+\.[0-9]+\.[0-9]+.*)$') { $AppVersion = $Matches[1].Trim() }
}

if (-not (Test-Path (Join-Path $appSrc 'SocialDownloader.exe'))) { Write-Host 'BLAD: brak app\SocialDownloader.exe'; exit 1 }
if (-not (Test-Path (Join-Path $engineSrc 'downloader.exe')))   { Write-Host 'BLAD: brak engine\downloader.exe'; exit 1 }

$DefaultTarget = Join-Path $env:LOCALAPPDATA 'Programs\SocialDownloader'

# ------------------------------------------------------------ helpers (bez UI)
function Test-DirWritable([string]$p) {
    try {
        New-Item -ItemType Directory -Force -Path $p -ErrorAction Stop | Out-Null
        $probe = Join-Path $p '.write-probe'
        [IO.File]::WriteAllText($probe, 'x'); Remove-Item $probe -Force
        return $true
    } catch { return $false }
}

function Kill-ProcessesUnder([string]$dir) {
    # upgrade: stopuj tylko procesy startowane z TEGO folderu (nie dotyka innych instalacji)
    try {
        Get-CimInstance Win32_Process -Filter "Name='downloader.exe' or Name='SocialDownloader.exe'" -ErrorAction SilentlyContinue |
            ForEach-Object {
                if ($_.ExecutablePath -and $_.ExecutablePath -like "$($dir.TrimEnd('\'))\*") {
                    Stop-Process -Id $_.ProcessId -Force -ErrorAction SilentlyContinue
                }
            }
    } catch { }
}

function Make-Launcher([string]$dest) {
    $vbsPath = Join-Path $dest 'SocialDownloader-Start.vbs'
    $vbs = 'Set sh = CreateObject("Wscript.Shell")' + "`r`n" +
           'sh.CurrentDirectory = "' + $dest + '"' + "`r`n" +
           'sh.Run "cmd /c ""' + $dest + '\START.bat""", 0, False' + "`r`n"
    [IO.File]::WriteAllText($vbsPath, $vbs, (New-Object System.Text.UTF8Encoding($false)))
    return $vbsPath
}

function Make-Shortcuts([string]$dest, [string]$vbsPath) {
    $ws = New-Object -ComObject WScript.Shell
    $targets = @(
        (Join-Path $env:APPDATA 'Microsoft\Windows\Start Menu\Programs\Social Downloader.lnk'),
        (Join-Path $ws.SpecialFolders('Desktop') 'Social Downloader.lnk')
    )
    foreach ($t in $targets) {
        $lnk = $ws.CreateShortcut($t)
        $lnk.TargetPath       = "$env:SystemRoot\System32\wscript.exe"
        $lnk.Arguments        = '"' + $vbsPath + '"'
        $lnk.IconLocation     = "$(Join-Path $dest 'app\SocialDownloader.exe'),0"
        $lnk.Description      = "Social Downloader $AppVersion (TheDAViL)"
        $lnk.WorkingDirectory = $dest
        $lnk.Save()
    }
}

function Make-Uninstaller([string]$dest) {
    $batPath = Join-Path $dest 'Odinstaluj.bat'
    $bat = '@echo off' + "`r`n" +
           'rem Social Downloader - odinstalowanie (TheDAViL)' + "`r`n" +
           'powershell -NoProfile -ExecutionPolicy Bypass -WindowStyle Hidden -File "%~dp0Odinstaluj.ps1" -Dir "%~dp0."' + "`r`n" +
           '(goto) 2>nul & rd /s /q "%~dp0"' + "`r`n"
    [IO.File]::WriteAllText($batPath, $bat, (New-Object System.Text.UTF8Encoding($false)))

    $psPath = Join-Path $dest 'Odinstaluj.ps1'
    $ps  = '# Auto-generated: Social Downloader odinstaluj' + "`r`n"
    $ps += "param([string]`$Dir)" + "`r`n"
    $ps += "`$ErrorActionPreference = 'SilentlyContinue'" + "`r`n"
    $ps += "`$Dir = (Resolve-Path `$Dir).Path.TrimEnd('\')" + "`r`n"
    $ps += "# 1. stop procesow startowanych z TEGO katalogu" + "`r`n"
    $ps += "Get-CimInstance Win32_Process -Filter `"Name='downloader.exe' or Name='SocialDownloader.exe'`" | ForEach-Object {" + "`r`n"
    $ps += "    if (`$_.ExecutablePath -and `$_.ExecutablePath -like `"*`$Dir*`") { Stop-Process -Id `$_.ProcessId -Force }" + "`r`n"
    $ps += '}' + "`r`n"
    $ps += "# 2. wpis 'Programy i funkcje' (HKCU)" + "`r`n"
    $ps += "Remove-Item -Path 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Uninstall\SocialDownloader' -Recurse -Force" + "`r`n"
    $ps += "# 3. skroty" + "`r`n"
    $ps += "Remove-Item (Join-Path `$env:APPDATA 'Microsoft\Windows\Start Menu\Programs\Social Downloader.lnk') -Force" + "`r`n"
    $ps += "try { `$ws = New-Object -ComObject WScript.Shell; Remove-Item (Join-Path `$ws.SpecialFolders('Desktop') 'Social Downloader.lnk') -Force } catch {}" + "`r`n"
    $ps += "# 4. regula firewall (bez admina: cicho odmawia)" + "`r`n"
    $ps += "netsh advfirewall firewall delete rule name='SocialDownloader-Engine' | Out-Null" + "`r`n"
    [IO.File]::WriteAllText($psPath, $ps, (New-Object System.Text.UTF8Encoding($true)))
}

function Write-ReadmeInstall([string]$dest) {
    $ln  = "Social Downloader $AppVersion - instalacja lokalna (kreator)"
    $txt = $ln + "`r`n" +
           ('=' * $ln.Length) + "`r`n" +
           'Start: skrot "Social Downloader" (Menu Start albo Pulpit) - uruchamia silnik' + "`r`n" +
           '(bez okna, 127.0.0.1:8765) i klienta GUI w jednym kliknieciu.' + "`r`n" + "`r`n" +
           'Struktura: app\ = klient (WPF .NET 8) | engine\ = silnik (Go + yt-dlp + ffmpeg)' + "`r`n" +
           'Odinstalowanie: "Programy i funkcje" -> Social Downloader (albo Odinstaluj.bat).' + "`r`n" + "`r`n" +
           'Licencja premium jest powiazana z HardwareID komputera.' + "`r`n" +
           'Pobieraj wylacznie tresci, na ktore masz prawa (regulaminy platform).' + "`r`n"
    [IO.File]::WriteAllText((Join-Path $dest 'README-INSTALACJA.txt'), $txt, (New-Object System.Text.UTF8Encoding($true)))
}

function Install-ToTarget([string]$dest, [scriptblock]$onPhase, [scriptblock]$onProgress, [bool]$fw) {
    $dest = $dest.TrimEnd('\')
    if (-not (Test-DirWritable $dest)) { throw "Folder docelowy nie jest zapisywalny: $dest" }
    if ($dest -ieq $env:ProgramFiles -or $dest -ieq ${env:ProgramFiles(x86)}) {
        throw 'Wybierz PODFOLDER (np. ...\Programs\SocialDownloader), nie sam katalog Program Files.'
    }
    if ($dest -ilike "$PSScriptRoot*") { throw 'Nie instaluj do katalogu pakietu instalatora.' }

    & $onPhase 'Sprawdzanie poprzedniej instalacji…'
    Kill-ProcessesUnder $dest

    & $onPhase 'Kopiowanie silnika…'
    New-Item -ItemType Directory -Force -Path (Join-Path $dest 'engine') | Out-Null
    New-Item -ItemType Directory -Force -Path (Join-Path $dest 'app') | Out-Null

    $srcs = @('downloader.exe','yt-dlp.exe','ffmpeg.exe','ffprobe.exe','public-key.pem','ads-config.json') |
        ForEach-Object { $p = Join-Path $engineSrc $_; if (Test-Path $p) { $p } }
    foreach ($s in $srcs) {
        Copy-Item $s -Destination (Join-Path $dest "engine\$(Split-Path -Leaf $s)") -Force
        & $onProgress 8
    }

    & $onPhase 'Kopiowanie aplikacji…'
    $appFiles = @(Get-ChildItem -Path $appSrc -Recurse -File)
    $total = [Math]::Max(1, $appFiles.Count)
    foreach ($d in ($appFiles | ForEach-Object { $_.DirectoryName } | Sort-Object -Unique)) {
        $relDir = $d.Substring($appSrc.Length).TrimStart('\')
        if ($relDir) { New-Item -ItemType Directory -Force -Path (Join-Path $dest "app\$relDir") | Out-Null }
    }
    $i = 0
    foreach ($f in $appFiles) {
        $i++
        $rel = $f.FullName.Substring($appSrc.Length).TrimStart('\')
        Copy-Item $f.FullName -Destination (Join-Path $dest "app\$rel") -Force
        if (($i % 20 -eq 0) -or ($i -eq $total)) {
            & $onProgress (8 + [int](72 * $i / $total))   # 8..80%
        }
    }
    & $onProgress 82

    & $onPhase 'Skróty i launcher…'
    if (Test-Path (Join-Path $stagingRoot 'START.bat')) {
        Copy-Item (Join-Path $stagingRoot 'START.bat') -Destination (Join-Path $dest 'START.bat') -Force
    }
    $vbs = Make-Launcher $dest
    if (Test-Path $logoSrc) { Copy-Item $logoSrc -Destination (Join-Path $dest 'SocialDownloader-logo.png') -Force }
    Make-Shortcuts $dest $vbs
    Make-Uninstaller $dest
    & $onProgress 90

    & $onPhase 'Firewall…'
    $fwResult = 'skip'
    if ($fw) {
        $out = & netsh advfirewall firewall add rule name='SocialDownloader-Engine' dir=in action=allow program="$(Join-Path $dest 'engine\downloader.exe')" enable=yes 2>&1
        if ($LASTEXITCODE -eq 0) { $fwResult = 'ok' } else { $fwResult = 'noadmin' }
    }
    & $onProgress 94

    & $onPhase 'Wpis w Programach i funkcjach…'
    $sizeKB = 0
    try { $sizeKB = [int]((Get-ChildItem -Path $dest -Recurse -File | Measure-Object Length -Sum).Sum / 1KB) } catch { }
    $uk = 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Uninstall\SocialDownloader'
    New-Item -Path $uk -Force | Out-Null
    $set = {
        param($n, $v, $t)
        if ($t) { Set-ItemProperty -Path $uk -Name $n -Value $v -Type $t }
        else    { Set-ItemProperty -Path $uk -Name $n -Value $v }
    }
    & $set 'DisplayName'     'Social Downloader'
    & $set 'DisplayVersion'  $AppVersion
    & $set 'Publisher'       'TheDAViL'
    & $set 'DisplayIcon'     (Join-Path $dest 'app\SocialDownloader.exe')
    & $set 'InstallLocation' $dest
    & $set 'URLInfoAbout'    'https://thedavil.idsl.pl'
    & $set 'EstimatedSize'   $sizeKB 'DWord'
    & $set 'NoModify'        1 'DWord'
    & $set 'NoRepair'        1 'DWord'
    & $set 'UninstallString' ('"' + (Join-Path $dest 'Odinstaluj.bat') + '"')
    & $set 'Comments'        'Facebook/Instagram/TikTok/YouTube (silnik lokalny 127.0.0.1:8765)'
    & $onProgress 97

    & $onPhase 'README…'
    Write-ReadmeInstall $dest
    & $onProgress 100

    return @{ Target = $dest; Version = $AppVersion; Firewall = $fwResult; Files = @(Get-ChildItem $dest -Recurse -File).Count }
}

# --------------------------------------------------------------------------- GUI
$Script:Win = $null
function Phase-Ui([string]$m) {
    $Script:Win.Dispatcher.Invoke([System.Windows.Threading.DispatcherPriority]::Background, [action]{ $Script:TbPhase.Text = $m })
}
function Prog-Ui([double]$p) {
    $Script:Win.Dispatcher.Invoke([System.Windows.Threading.DispatcherPriority]::Background, [action]{ $Script:Bar.Value = $p })
}

$xaml = @"
<Window xmlns="http://schemas.microsoft.com/winfx/2006/xaml/presentation"
        xmlns:x="http://schemas.microsoft.com/winfx/2006/xaml"
        Title="Social Downloader - Instalator" Width="560" Height="470"
        WindowStartupLocation="CenterScreen" ResizeMode="CanMinimize"
        Background="#FF0A0A0A" FontFamily="Segoe UI">
  <Grid Margin="24,18,24,14">
    <Grid.RowDefinitions>
      <RowDefinition Height="Auto"/>
      <RowDefinition Height="*"/>
      <RowDefinition Height="Auto"/>
    </Grid.RowDefinitions>

    <StackPanel Orientation="Horizontal" Grid.Row="0" Margin="0,0,0,12">
      <Image x:Name="Logo" Width="40" Height="40"/>
      <StackPanel Margin="12,3,0,0" VerticalAlignment="Center">
        <TextBlock Text="SOCIAL DOWNLOADER" FontSize="19" FontWeight="Bold" Foreground="#FFF2F0EC"/>
        <TextBlock x:Name="TxtSub" FontSize="11" Foreground="#FF726C66"/>
      </StackPanel>
    </StackPanel>

    <Grid x:Name="PnlWelcome" Grid.Row="1">
      <StackPanel>
      <TextBlock Text="Ten kreator zainstaluje Social Downloader na tym komputerze." FontSize="13" Foreground="#FFEAEAEC" TextWrapping="Wrap"/>
      <TextBlock Text="• aplikacja (WPF) + lokalny silnik (Go, yt-dlp, ffmpeg) — 127.0.0.1:8765" FontSize="11.5" Foreground="#FFA09A94" Margin="0,10,0,0" TextWrapping="Wrap"/>
      <TextBlock Text="• skróty w Menu Start i na pulpicie — start jednym kliknięciem" FontSize="11.5" Foreground="#FFA09A94" Margin="0,3,0,0" TextWrapping="Wrap"/>
      <TextBlock Text="• odinstalowanie z „Programów i funkcji” albo Odinstaluj.bat" FontSize="11.5" Foreground="#FFA09A94" Margin="0,3,0,0" TextWrapping="Wrap"/>
      <TextBlock Text="Folder docelowy:" FontSize="12.5" Foreground="#FFEAEAEC" Margin="0,20,0,6"/>
      <Grid>
        <Grid.ColumnDefinitions><ColumnDefinition Width="*"/><ColumnDefinition Width="Auto"/></Grid.ColumnDefinitions>
        <TextBox x:Name="TbTarget" Height="28" FontSize="12.5" VerticalContentAlignment="Center"
                 Background="#FF141414" Foreground="#FFEAEAEC" BorderBrush="#FF303030" Padding="6,0"/>
        <Button x:Name="BtnBrowse" Grid.Column="1" Content="Przeglądaj…" Width="112" Height="28" Margin="8,0,0,0"/>
      </Grid>
      <CheckBox x:Name="CbFirewall" IsChecked="False" Margin="0,12,0,0">
        <TextBlock Text="Reguła firewall dla silnika (tryb LAN) — wymaga uprawnień administratora"
                   TextWrapping="Wrap" FontSize="11" Foreground="#FFA09A94"/>
      </CheckBox>
      <TextBlock x:Name="ErrTb" Text="" Foreground="#FFFF5F6F" FontSize="11" TextWrapping="Wrap" Margin="0,10,0,0"/>
      </StackPanel>
    </Grid>

    <StackPanel x:Name="PnlBusy" Grid.Row="1" Visibility="Collapsed">
      <TextBlock x:Name="TbPhase" Text="Przygotowanie…" FontSize="13.5" Foreground="#FFEAEAEC" Margin="0,24,0,12"/>
      <ProgressBar x:Name="Bar" Height="8" Minimum="0" Maximum="100"/>
      <TextBlock x:Name="TbDetail" Text="" FontSize="10.5" Foreground="#FF726C66" Margin="0,8,0,0"/>
    </StackPanel>

    <Grid x:Name="PnlDone" Grid.Row="1" Visibility="Collapsed">
      <StackPanel>
        <TextBlock Text="✔ Zainstalowano" FontSize="17" FontWeight="Bold" Foreground="#FF3DDC84" Margin="0,8,0,0"/>
        <TextBlock x:Name="TxtDone" FontSize="11.5" Foreground="#FFA09A94" TextWrapping="Wrap" Margin="0,12,0,0"/>
      </StackPanel>
    </Grid>

    <DockPanel Grid.Row="2" Margin="0,12,0,0">
      <Button x:Name="BtnClose"   DockPanel.Dock="Right" Content="Anuluj"     Width="96"  Height="30" Background="#FF262626" Foreground="#FFEAEAEC" BorderThickness="0" Cursor="Hand"/>
      <Button x:Name="BtnInstall" DockPanel.Dock="Right" Content="Zainstaluj" Width="124" Height="30" Margin="0,0,8,0" Background="#FFE2172C" Foreground="#FFFFFFFF" BorderThickness="0" Cursor="Hand" FontWeight="SemiBold"/>
    </DockPanel>
  </Grid>
</Window>
"@

try {
    $Script:Win = [System.Windows.Markup.XamlReader]::Parse($xaml)
} catch {
    Write-Host "BLAD XAML: $($_.Exception.Message)"
    exit 3
}
$win = $Script:Win

foreach ($n in @('Logo','TxtSub','PnlWelcome','PnlBusy','PnlDone','TbTarget','BtnBrowse','CbFirewall','ErrTb','TbPhase','Bar','TbDetail','TxtDone','BtnClose','BtnInstall')) {
    Set-Variable -Name $n -Value $win.FindName($n) -Scope Script
}
$Script:TxtSub.Text    = "Instalator - wersja $AppVersion (TheDAViL)"
$Script:TbTarget.Text  = $DefaultTarget
if ($Target) { $Script:TbTarget.Text = [Environment]::ExpandEnvironmentVariables($Target) }
if (Test-Path $logoSrc) {
    $Script:Logo.Source = New-Object System.Windows.Media.Imaging.BitmapImage((New-Object System.Uri($logoSrc)))
    $Script:Win.Icon    = New-Object System.Windows.Media.Imaging.BitmapImage((New-Object System.Uri($logoSrc)))
}

$Script:BtnBrowse.Add_Click({
    $dlg = New-Object System.Windows.Forms.FolderBrowserDialog
    $dlg.Description   = 'Wybierz folder instalacji Social Downloader'
    $dlg.SelectedPath  = $Script:TbTarget.Text
    if ($dlg.ShowDialog() -eq [System.Windows.Forms.DialogResult]::OK) {
        $Script:TbTarget.Text = $dlg.SelectedPath
    }
})

$Script:BtnClose.Add_Click({ $win.Close() })

$Script:BtnInstall.Add_Click({
    try {
        $Script:ErrTb.Text = ''
        $dest = ([IO.Path]::GetFullPath(($Script:TbTarget.Text.TrimEnd('\') + '\'))).TrimEnd('\')
        $wantFw = ($Script:CbFirewall.IsChecked -eq $true)
        $Script:PnlWelcome.Visibility  = [System.Windows.Visibility]::Collapsed
        $Script:PnlBusy.Visibility     = [System.Windows.Visibility]::Visible
        $Script:BtnInstall.IsEnabled   = $false
        $Script:BtnBrowse.IsEnabled    = $false
        $Script:TbTarget.IsEnabled     = $false
        $Script:CbFirewall.IsEnabled   = $false

        $result = Install-ToTarget $dest { param($m) Phase-Ui $m } { param($p) Prog-Ui $p } $wantFw

        $fwTxt = switch ($result.Firewall) {
            'ok'      { 'reguła firewall: dodana' }
            'noadmin' { 'reguła firewall: pominięta (brak uprawnień administratora — dodaj ręcznie dla trybu LAN)' }
            default   { 'reguła firewall: pominięta (niezaznaczona)' }
        }
        $Script:TxtDone.Text = "Social Downloader $($result.Version) w $($result.Target)`r`n" +
            "• $fwTxt`r`n" +
            "• $($result.Files) plików przezinstalowane`r`n`r`n" +
            'Uruchom: skrót „Social Downloader" (Menu Start albo Pulpit).' + "`r`n" +
            'Silnik ruszy razem z klientem (cicho, 127.0.0.1:8765).'
        $Script:PnlBusy.Visibility  = [System.Windows.Visibility]::Collapsed
        $Script:PnlDone.Visibility  = [System.Windows.Visibility]::Visible
        $Script:BtnInstall.Visibility = [System.Windows.Visibility]::Collapsed
        $Script:BtnClose.Content    = 'Zakończ'
    } catch {
        $Script:PnlBusy.Visibility   = [System.Windows.Visibility]::Collapsed
        $Script:PnlWelcome.Visibility = [System.Windows.Visibility]::Visible
        $Script:ErrTb.Text = $_.Exception.Message
        $Script:BtnInstall.IsEnabled = $true
        $Script:BtnBrowse.IsEnabled  = $true
        $Script:TbTarget.IsEnabled   = $true
        $Script:CbFirewall.IsEnabled = $true
    }
})

if ($SelfTest) {
    $win.Show()
    $win.Dispatcher.Invoke([System.Windows.Threading.DispatcherPriority]::Background, [action]{})
    Start-Sleep -Milliseconds 800
    $win.Close()
    Write-Host 'SELFTEST-OK'
    exit 0
}

[void]$win.ShowDialog()
exit 0
