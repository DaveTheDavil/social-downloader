using System;
using System.Collections.Generic;
using System.Collections.ObjectModel;
using System.ComponentModel;
using System.IO;
using System.Linq;
using System.Runtime.CompilerServices;
using System.Threading.Tasks;
using System.Windows.Input;
using SocialDownloader.Models;
using SocialDownloader.Services;

namespace SocialDownloader.ViewModels;

public class RelayCommand : ICommand
{
    private readonly Action<object?> _execute;
    private readonly Func<object?, bool>? _canExecute;

    public RelayCommand(Action<object?> execute, Func<object?, bool>? canExecute = null)
    {
        _execute = execute;
        _canExecute = canExecute;
    }

    public bool CanExecute(object? parameter) => _canExecute?.Invoke(parameter) ?? true;
    public void Execute(object? parameter) => _execute(parameter);
    public event EventHandler? CanExecuteChanged;
    public void RaiseCanExecuteChanged() => CanExecuteChanged?.Invoke(this, EventArgs.Empty);
}

/// <summary>
/// Jedna pozycja do pobrania na liście klienta (element serii IG lub całość).
/// </summary>
public class DownloadItem : INotifyPropertyChanged
{
    private bool _selected = true;

    public int Index { get; set; }
    public string Title { get; set; } = string.Empty;
    public bool IsImage { get; set; }
    public int Duration { get; set; }
    public List<int> Qualities { get; set; } = new();
    public List<Format> Formats { get; set; } = new();
    public string Label { get; set; } = string.Empty;

    /// <summary>Kiedy client zaznaczy/odznaczy → odśwież sąsiednie property VM.</summary>
    public Action? SelectionChanged { get; set; }

    public bool Selected
    {
        get => _selected;
        set { _selected = value; OnPropertyChanged(); SelectionChanged?.Invoke(); }
    }

    public string TypeLabel => IsImage ? "Obraz" : "Wideo";

    public event PropertyChangedEventHandler? PropertyChanged;

    private void OnPropertyChanged([CallerMemberName] string? name = null)
        => PropertyChanged?.Invoke(this, new PropertyChangedEventArgs(name));
}

public class MainViewModel : INotifyPropertyChanged
{
    private readonly EngineClient _engine;
    private readonly LicenseService _license;
    private readonly AdService _ads;

    private string _url = string.Empty;
    private VideoInfo? _videoInfo;
    private int _selectedQuality;
    private bool _isDownloading;
    private int _progress;
    private string _status = "Wklej link do filmu/zdjęcia z FB, IG, TT lub YT.";
    private string _error = string.Empty;
    private AdUnit? _bannerAd;
    private AdUnit? _sidebarAd;
    private AdUnit? _sidebarAd2;
    private AdUnit? _inlineAd;
    private AdUnit? _interstitialAd;
    private bool _isPremium;
    private string _outputFolder = string.Empty;
    private ObservableCollection<DownloadItem>? _items;
    private string _playlistNote = string.Empty;
    private List<DownloadItem>? _pendingBatch;
    private string _igCookieInput = string.Empty;
    private bool _igSessionConfigured;

    public event EventHandler? InterstitialRequested;
    public event EventHandler? DownloadCompleted;

    public MainViewModel(EngineClient engine, LicenseService license, AdService ads)
    {
        _engine = engine;
        _license = license;
        _ads = ads;

        AnalyzeCommand = new RelayCommand(_ => AnalyzeAsync(), _ => !string.IsNullOrWhiteSpace(Url) && !IsDownloading);
        DownloadCommand = new RelayCommand(_ => DownloadAsync(), _ => SelectedCount > 0 && !IsDownloading && !string.IsNullOrWhiteSpace(OutputFolder));
        ActivateLicenseCommand = new RelayCommand(_ => ActivateLicense(), _ => !IsDownloading);
        ChooseFolderCommand = new RelayCommand(_ => ChooseFolder(), _ => !IsDownloading);
        SaveIgCookieCommand = new RelayCommand(_ => SaveIgCookieAsync(), _ => !IsDownloading);
        ClearIgCookieCommand = new RelayCommand(_ => ClearIgCookieAsync(), _ => !IsDownloading);

        // Stan sesji IG odpytany przy starcie (silnik zna plik ig-cookie.txt).
        _ = RefreshIgSessionAsync();

        // Reklamy widoczne od razu przy starcie (bez czekania na pierwszą analizę);
        // przy premium-na-pliku sloty nie startują, a /info odświeża je po każdej analizie.
        _ = LoadStartupAdsAsync();

        // Domyślny folder docelowy — użytkownik może tam od razu pobierać bez wybierania katalogu.
        var defFolder = Path.Combine(
            Environment.GetFolderPath(Environment.SpecialFolder.UserProfile),
            "Downloads", "SocialDownloader");
        try { if (!Directory.Exists(defFolder)) Directory.CreateDirectory(defFolder); OutputFolder = defFolder; }
        catch { /* w najgorszym razie folder zostaje pusty i użytkownik wybierze ręcznie */ }
    }

    public string Url
    {
        get => _url;
        set { _url = value; OnPropertyChanged(); ((RelayCommand)AnalyzeCommand).RaiseCanExecuteChanged(); }
    }

    public VideoInfo? VideoInfo
    {
        get => _videoInfo;
        set
        {
            _videoInfo = value;
            OnPropertyChanged();
            OnPropertyChanged(nameof(HasQualities));
            OnPropertyChanged(nameof(Qualities));
            // Nowa analiza = stare zaznaczenie jakości tracą sens.
            _selectedQuality = 0;
            OnPropertyChanged(nameof(SelectedQuality));
        }
    }

    public bool HasQualities => VideoInfo != null && VideoInfo.Qualities != null && VideoInfo.Qualities.Count > 0;

    public List<int> Qualities => VideoInfo?.Qualities ?? new List<int>();

    /// <summary>Rozdzielczość wybrana w ComboBox — dotyczy pozycji wideo na liście.</summary>
    public int SelectedQuality
    {
        get => _selectedQuality;
        set { _selectedQuality = value; OnPropertyChanged(); }
    }

    public ObservableCollection<DownloadItem>? Items
    {
        get => _items;
        set { _items = value; OnPropertyChanged(); OnPropertyChanged(nameof(HasItems)); OnPropertyChanged(nameof(DownloadButtonText)); ((RelayCommand)DownloadCommand).RaiseCanExecuteChanged(); }
    }

    public bool HasItems => Items != null && Items.Count > 0;

    public string PlaylistNote
    {
        get => _playlistNote;
        set { _playlistNote = value; OnPropertyChanged(); }
    }

    /// <summary>Etykieta przycisku pobierania zależna od liczby zaznaczonych pozycji.</summary>
    public string DownloadButtonText
    {
        get
        {
            var n = SelectedCount;
            return n > 1 ? $"Pobierz zaznaczone ({n})" : "Pobierz plik";
        }
    }

    private int SelectedCount => Items?.Count(x => x.Selected) ?? 0;

    public bool IsDownloading
    {
        get => _isDownloading;
        set
        {
            _isDownloading = value; OnPropertyChanged();
            ((RelayCommand)AnalyzeCommand).RaiseCanExecuteChanged();
            ((RelayCommand)DownloadCommand).RaiseCanExecuteChanged();
            ((RelayCommand)ActivateLicenseCommand).RaiseCanExecuteChanged();
            ((RelayCommand)ChooseFolderCommand).RaiseCanExecuteChanged();
        }
    }

    public int Progress
    {
        get => _progress;
        set { _progress = Math.Clamp(value, 0, 100); OnPropertyChanged(); }
    }

    public string Status
    {
        get => _status;
        set { _status = value; OnPropertyChanged(); }
    }

    public string Error
    {
        get => _error;
        set { _error = value; OnPropertyChanged(); }
    }

    public AdUnit? BannerAd
    {
        get => _bannerAd;
        set { _bannerAd = value; OnPropertyChanged(); OnPropertyChanged(nameof(ShowLeaderboard)); }
    }

    /// <summary>Medium Rectangle 300×250 — drugi slot (sidebar treści).</summary>
    public AdUnit? SidebarAd
    {
        get => _sidebarAd;
        set { _sidebarAd = value; OnPropertyChanged(); OnPropertyChanged(nameof(ShowSidebar)); OnPropertyChanged(nameof(ShowSidebar2)); OnPropertyChanged(nameof(ShowRightRail)); }
    }

    /// <summary>Medium Rectangle 300×250 (2) — dolny slot prawej szyny; duplikat → slot milczy.</summary>
    public AdUnit? SidebarAd2
    {
        get => _sidebarAd2;
        set { _sidebarAd2 = value; OnPropertyChanged(); OnPropertyChanged(nameof(ShowSidebar2)); OnPropertyChanged(nameof(ShowRightRail)); }
    }

    /// <summary>In-content 468×60 — trzeci, dyskretny slot; wyświetlany tylko gdy nie-duplikat.</summary>
    public AdUnit? InlineAd
    {
        get => _inlineAd;
        set { _inlineAd = value; OnPropertyChanged(); OnPropertyChanged(nameof(ShowInline)); }
    }

    public AdUnit? InterstitialAd
    {
        get => _interstitialAd;
        set { _interstitialAd = value; OnPropertyChanged(); }
    }

    public bool IsPremium
    {
        get => _isPremium;
        set
        {
            _isPremium = value;
            OnPropertyChanged();
            OnPropertyChanged(nameof(PremiumLabel));
            OnPropertyChanged(nameof(ShowAds));
            OnPropertyChanged(nameof(ShowLeaderboard));
            OnPropertyChanged(nameof(ShowSidebar));
            OnPropertyChanged(nameof(ShowSidebar2));
            OnPropertyChanged(nameof(ShowInline));
            OnPropertyChanged(nameof(ShowRightRail));
        }
    }

    public string PremiumLabel => IsPremium ? "Tak" : "Nie";

    public bool ShowAds => !IsPremium;

    // Widoczność slotów: darmowa wersja + kreatywa pod ręką
    public bool ShowLeaderboard => ShowAds && BannerAd != null;
    public bool ShowSidebar => ShowAds && SidebarAd != null;
    public bool ShowSidebar2 => ShowAds && SidebarAd2 != null
        // dedupe: drugi rect nie powtarza kreatywy z pierwszego
        && !string.Equals(SidebarAd2?.Id, SidebarAd?.Id, StringComparison.Ordinal);
    public bool ShowInline => ShowAds && InlineAd != null;

    /// <summary>Prawa szyna istnieje, gdy ma cokolwiek pokazać (steruje szerokością kolumny).</summary>
    public bool ShowRightRail => ShowAds && (SidebarAd != null || SidebarAd2 != null);

    public string OutputFolder
    {
        get => _outputFolder;
        set { _outputFolder = value; OnPropertyChanged(); ((RelayCommand)DownloadCommand).RaiseCanExecuteChanged(); }
    }

    // ----- Sesja Instagram (cookie) -----

    /// <summary>Pole tekstowe klienta — wklejony cookie sesji IG (przechowywany tylko tu).</summary>
    public string IgCookieInput
    {
        get => _igCookieInput;
        set { _igCookieInput = value; OnPropertyChanged(); }
    }

    /// <summary>Czy silnik ma zapisaną sesję IG (plik ig-cookie.txt).</summary>
    public bool IgSessionConfigured
    {
        get => _igSessionConfigured;
        private set { _igSessionConfigured = value; OnPropertyChanged(); OnPropertyChanged(nameof(IgSessionLabel)); }
    }

    /// <summary>Status label panelu sesji; zawsze widoczny (podmiana/czyszczenie cookie).</summary>
    public string IgSessionLabel
    {
        get
        {
            return IgSessionConfigured
                ? "Sesja IG: aktywna (pełne serie)"
                : "Sesja IG: nie ustawiona (anonimowo — tylko pierwszy element serii)";
        }
    }

    /// <summary>Stan z silnika; null → silnik nieosiągalny (label zostaje „nie ustawiona").</summary>
    private async Task<bool?> RefreshIgSessionAsync()
    {
        var conf = await _engine.GetIgCookieConfiguredAsync();
        if (conf.HasValue)
        {
            IgSessionConfigured = conf.Value;
        }
        return conf;
    }

    /// <summary>
    /// Zapis sesji IG wg IgCookieInput. Puste pole NIE kasuje już zapisanej sesji
    /// (dawne zachowanie: klik „Zapisz" z pustym polem = wymaz sesji → tryb anonimowy).
    /// Czyszczenie = jawnie przyciskiem „Wyczyść" (ClearIgCookieAsync) z potwierdzeniem.
    /// </summary>
    public async void SaveIgCookieAsync()
    {
        Error = string.Empty;
        var cookie = (IgCookieInput ?? "").Trim();
        if (cookie.Length == 0)
        {
            // Cookie NIE jest logowane ani pokazywane — tylko zapis do silnika (plik ig-cookie.txt).
            Status = "Wklej cookie IG w pole (jest puste) — czyszczenie jest pod osobnym przyciskiem „Wyczyść”.";
            return;
        }
        await PersistIgCookieAsync(cookie);
    }

    /// <summary>Jawne czyszczenie sesji IG (przycisk „Wyczyść") — z potwierdzeniem, bo akcja destrukcyjna.</summary>
    public async void ClearIgCookieAsync()
    {
        Error = string.Empty;
        if (!IgSessionConfigured)
        {
            Status = "Brak zapisanej sesji IG — nic do wyczyszczenia.";
            return;
        }
        var answer = System.Windows.MessageBox.Show(
            "Wyczyścić zapisaną sesję Instagram?\n\nPełne serie zdjęć będą niedostępne (tryb anonimowy).",
            "Sesja IG",
            System.Windows.MessageBoxButton.YesNo,
            System.Windows.MessageBoxImage.Question,
            System.Windows.MessageBoxResult.No);
        if (answer != System.Windows.MessageBoxResult.Yes)
        {
            Status = "Czyszczenie anulowane — sesja IG nadal aktywna.";
            return;
        }
        await PersistIgCookieAsync("");
    }

    /// <summary>Wspólny POST /ig-cookie (zapis LUB czyszczenie) + odświeżenie labelki stanu.</summary>
    private async Task PersistIgCookieAsync(string cookie)
    {
        // Cookie NIE jest logowane ani pokazywane — tylko zapis do silnika (plik ig-cookie.txt).
        Status = cookie.Length == 0 ? "Czyszczenie sesji Instagram..." : "Zapisywanie sesji Instagram...";
        var ok = await _engine.SetIgCookieAsync(cookie);
        await RefreshIgSessionAsync();
        if (ok)
        {
            if (cookie.Length > 0) IgCookieInput = ""; // ciasteczka nie zostają w polu tekstowym
            Status = cookie.Length == 0
                ? "Sesja Instagram wyczyszczona — tryb anonimowy."
                : "Sesja Instagram zapisana (ig-cookie.txt) — pełne serie zdjęć dostępne.";
        }
        else
        {
            Status = cookie.Length == 0 ? "Nie udało się wyczyścić sesji Instagram." : "Nie udało się zapisać sesji Instagram.";
            Error = string.IsNullOrEmpty(_engine.LastError)
                ? "Silnik nie potwierdził operacji (nieoczekiwana odpowiedź) — spróbuj ponownie."
                : _engine.LastError;
        }
    }

    public ICommand AnalyzeCommand { get; }
    public ICommand DownloadCommand { get; }
    public ICommand ActivateLicenseCommand { get; }
    public ICommand ChooseFolderCommand { get; }
    public ICommand SaveIgCookieCommand { get; }
    public ICommand ClearIgCookieCommand { get; }

    public async void AnalyzeAsync()
    {
        Error = string.Empty;
        Progress = 0;
        Status = "Analizowanie linku...";
        IsDownloading = true;

        // Reset panelu, by nie pokazywać danych z POPRZEDNIEJ analizy, gdy ta się nie powiedzie.
        Items = null;
        PlaylistNote = "";
        VideoInfo = null;

        try
        {
            var info = await _engine.GetInfoAsync(Url);
            if (info == null)
            {
                if (string.IsNullOrEmpty(_engine.LastError))
                    Error = "Nie można pobrać informacji o tym linku. Sprawdź URL i spróbuj ponownie.";
                else
                    Error = _engine.LastError;
                Status = "Błąd analizy.";
                return;
            }

            VideoInfo = info;
            IsPremium = info.Premium;
            PlaylistNote = info.PlaylistNote ?? "";
            BuildItems();

            if (IsPremium)
            {
                Status = info.IsPlaylist
                    ? $"Znaleziono {Items?.Count ?? 0} pozycji. Wersja premium aktywna."
                    : "Wersja premium aktywna – brak reklam, pełna rozdzielczość.";
            }
            else
            {
                Status = "Tryb darmowy – dostępne do 720p. Reklamy są wyświetlane.";
            }

            if (!IsPremium)
            {
                _ = LoadAdsAsync();
            }
        }
        catch (Exception ex)
        {
            Error = ex.Message;
            Status = "Błąd.";
        }
        finally
        {
            IsDownloading = false;
        }
    }

    /// <summary>
    /// Buduje listę pozycji do pobrania z odpowiedzi /info:
    /// seria → elementy; pojedyncze w → jedna pozycja pseudo-entry (stale checked).
    /// </summary>
    private void BuildItems()
    {
        var coll = new ObservableCollection<DownloadItem>();
        List<EntryInfo> entries;
        if (VideoInfo != null && VideoInfo.IsPlaylist && VideoInfo.Entries != null && VideoInfo.Entries.Count > 0)
        {
            entries = VideoInfo.Entries;
        }
        else if (VideoInfo != null)
        {
            entries = new List<EntryInfo>
            {
                new EntryInfo
                {
                    Index = 1,
                    Id = VideoInfo.Id,
                    Title = VideoInfo.Title,
                    Duration = VideoInfo.Duration,
                    IsImage = false,
                    Qualities = VideoInfo.Qualities ?? new List<int>(),
                    Formats = VideoInfo.Formats ?? new List<Format>(),
                }
            };
        }
        else
        {
            entries = new List<EntryInfo>();
        }

        foreach (var e in entries)
        {
            var item = new DownloadItem
            {
                Index = e.Index,
                Title = e.Title,
                IsImage = e.IsImage,
                Duration = e.Duration,
                Qualities = e.Qualities ?? new List<int>(),
                Formats = e.Formats ?? new List<Format>(),
                Selected = true,
                Label = $"{e.Index}. {e.Title}" + (e.IsImage ? " (obraz)" : ""),
            };
            item.SelectionChanged = () =>
            {
                ((RelayCommand)DownloadCommand).RaiseCanExecuteChanged();
                OnPropertyChanged(nameof(DownloadButtonText));
            };
            coll.Add(item);
        }

        Items = coll;
    }

    /// <summary>Startup: najpierw licencja (engine /license), potem decyzja o slotos-store — bez mismatch WPF-pliku licencji.</summary>
    private async Task LoadStartupAdsAsync()
    {
        var premium = await RefreshLicenseAsync();
        if (premium) return;
        await LoadAdsAsync();
    }

    /// <summary>Synchronizuje IsPremium z silnikiem (/license) — zwraca stan premium.</summary>
    private async Task<bool> RefreshLicenseAsync()
    {
        try
        {
            var info = await _engine.GetLicenseAsync();
            IsPremium = info?.Premium ?? false;
            return IsPremium;
        }
        catch
        {
            return false; // silnik nieosiągalny → traktujemy jako darmowy tryb fail-safe
        }
    }

    private async Task LoadAdsAsync()
    {
        try
        {
            // Silnik losuje wagowo kreatywę przy KAŻDYM wywołaniu — trzy pytania dają
            // szansę na różne kreatywy w trzech slotach. Duplikat → slot milczy
            // ("nie pażernie": nie stawiamy tej samej kreatywy dwukrotnie na ekranie).
            var first = await _ads.GetBannerAdAsync();
            var second = await _ads.GetBannerAdAsync();
            var third = await _ads.GetBannerAdAsync();
            var fourth = await _ads.GetBannerAdAsync();

            BannerAd = first;
            SidebarAd = second;
            SidebarAd2 = fourth != null && fourth.Id != first?.Id && fourth.Id != second?.Id && fourth.Id != third?.Id ? fourth : null;
            InlineAd = third != null && third.Id != first?.Id && third.Id != second?.Id ? third : null;
        }
        catch
        {
            BannerAd = null;
            SidebarAd = null;
            SidebarAd2 = null;
            InlineAd = null;
        }
#if DEBUG
        // Maszyna developerska ma lokalną premkę → pula pusta; w DEBUG doklejamy
        // kreatywy demo, żeby dało się sprawdzić układ slotów. Release: nigdy.
        if (BannerAd == null && SidebarAd == null)
        {
            BannerAd = new AdUnit { Id = "demo-lb", Advertiser = "Sklep Omega (demo)", Title = "Leaderboard 728×90 — kreatywa demo", Description = "Elektronika, RTV, AGD w najlepszych cenach." };
            SidebarAd = new AdUnit { Id = "demo-rect", Advertiser = "GraX Studio (demo)", Title = "Medium Rectangle 300×250", Description = "Zbuduj imperium i podbij świat." };
            SidebarAd2 = new AdUnit { Id = "demo-rect2", Advertiser = "VPN Secure (demo)", Title = "Medium Rectangle 300×250 (2)", Description = "Chroń prywatność w sieci — szyfrowanie wszystkich urządzeń." };
            InlineAd = new AdUnit { Id = "demo-inline", Advertiser = "Nowa Era RTV (demo)", Title = "In-content 468×60 — kreatywa demo", Description = "Strefa okazji tygodnia." };
        }
#endif
    }

    /// <summary>
    /// Dobiera format dla pozycji: dokładnie jeden format "img"/"vid" (elementy
    /// IG z resolvera sesyjnego/anonimowego) → zwrócony W PROSTO, bez liczenia
    /// wysokości; obraz → format syntetyczny "img"; wideo yt-dlp → pasująca
    /// wysokość (SelectedQuality), a gdy brak — najbliższa ≤ / największa.
    /// </summary>
    private Format? ResolveForItem(DownloadItem it)
    {
        if (it.Formats is { Count: 1 } && (it.Formats[0].FormatId == "img" || it.Formats[0].FormatId == "vid"))
            return it.Formats[0]; // pełna jakość elementu IG — brak wyboru

        if (it.IsImage)
            return it.Formats?.FirstOrDefault() ?? new Format { FormatId = "img", Ext = "jpg" };

        var qs = SelectedQuality;
        return it.Formats?.FirstOrDefault(f => f.Height == qs)
            ?? it.Formats?.Where(f => f.Height > 0 && f.Height <= qs).OrderByDescending(f => f.Height).FirstOrDefault()
            ?? it.Formats?.Where(f => f.Height > 0).OrderByDescending(f => f.Height).FirstOrDefault()
            ?? it.Formats?.FirstOrDefault();
    }

    /// <summary>
    /// Format wprost ("img"/"vid" jako jedyny) — element IG pobierany prosto
    /// z CDN silnikiem; nie podlega bramce HD po stronie klienta (obrazy nie
    /// są strumieniami wideo, a sesyjne "vid" ma pełną jakość).
    /// </summary>
    private static bool IsDirectIgFormat(DownloadItem x) =>
        x.Formats is { Count: 1 } && (x.Formats[0].FormatId == "img" || x.Formats[0].FormatId == "vid");

    public async void DownloadAsync()
    {
        if (Items == null || SelectedCount == 0 || string.IsNullOrWhiteSpace(OutputFolder) || VideoInfo == null)
            return;

        var selected = Items.Where(x => x.Selected).ToList();

        // YouTube: twardy limit 10 min na wybrane pozycje wideo.
        var isYt = Url.Contains("youtube.com") || Url.Contains("youtu.be");
        if (isYt && selected.Any(x => !x.IsImage && x.Duration > 600))
        {
            Error = "YouTube: filmy powyżej 10 minut są zablokowane (unikanie longplayów).";
            return;
        }

        // Darmowa wersja: HD >720p tylko z premium. Dotyczy ścieżki yt-dlp —
        // elementy IG pobierane wprost ("img"/"vid" jako jedyny format) są
        // zwolnione (silnik też nie stoi bramki dla directURL).
        if (!IsPremium && selected.Any(x => !x.IsImage && !IsDirectIgFormat(x) && (ResolveForItem(x)?.Height ?? 0) > 720))
        {
            Error = "Rozdzielczość powyżej 720p wymaga wersji premium.";
            return;
        }

        // Interstitial raz przed całą partią (darmowa wersja).
        if (!IsPremium)
        {
            var interstitial = await _ads.GetInterstitialAdAsync();
            if (interstitial != null)
            {
                InterstitialAd = interstitial;
                _pendingBatch = selected;
                InterstitialRequested?.Invoke(this, EventArgs.Empty);
                return;
            }
        }

        _pendingBatch = selected;
        await RunBatchAsync();
    }

    public void OnInterstitialClosed()
    {
        InterstitialAd = null;
        if (_pendingBatch is { Count: > 0 })
            _ = RunBatchAsync();
    }

    private async Task RunBatchAsync()
    {
        var batch = _pendingBatch ?? new List<DownloadItem>();
        _pendingBatch = null;
        if (batch.Count == 0) return;

        Error = string.Empty;
        Progress = 0;
        IsDownloading = true;

        var failures = new List<string>();
        var successes = 0;

        try
        {
            for (var i = 0; i < batch.Count; i++)
            {
                var it = batch[i];
                var statusMarker = batch.Count > 1 ? $"{i + 1}/{batch.Count}: " : "";
                Status = $"Pobieranie {statusMarker}{it.Title}";

                var format = ResolveForItem(it);
                if (format == null || string.IsNullOrWhiteSpace(format.FormatId))
                {
                    failures.Add($"{it.Title}: nie znaleziono formatu");
                    continue;
                }

                // Nazwy unikalne w batchu (2 tytuły mogą być identyczne → overwrite).
                var fileName = batch.Count > 1
                    ? $"{it.Title}_{i + 1:D2}.{format.Ext}"
                    : $"{it.Title}.{format.Ext}";
                fileName = string.Join("_", fileName.Split(Path.GetInvalidFileNameChars()));
                var outPath = Path.Combine(OutputFolder, fileName);

                var jobId = await _engine.StartDownloadAsync(Url, format.FormatId, outPath, it.Index);
                if (string.IsNullOrEmpty(jobId))
                {
                    failures.Add($"{it.Title}: {(_engine.LastError is { Length: > 0 } e ? e : "nie można rozpocząć pobierania")}");
                    continue;
                }

                var ok = await PollJobAsync(jobId, statusMarker);
                if (ok)
                {
                    successes++;
                }
                else
                {
                    failures.Add($"{it.Title}: {Error}");
                }
            }

            Status = failures.Count == 0
                ? (successes > 1 ? $"Zakończono: {successes} plików w {OutputFolder}" : $"Zakończono: {OutputFolder}")
                : $"Gotowe ({successes}/{batch.Count}) — błędy: {failures.Count}";
            if (failures.Count > 0)
                Error = string.Join("\n", failures);
            if (successes > 0)
                DownloadCompleted?.Invoke(this, EventArgs.Empty);
        }
        catch (Exception ex)
        {
            Error = ex.Message;
            Status = "Błąd pobierania.";
        }
        finally
        {
            IsDownloading = false;
        }
    }

    /// <summary>Odpytuje /progress aż do stanu końcowego; true = ukończono.</summary>
    private async Task<bool> PollJobAsync(string jobId, string marker)
    {
        while (true)
        {
            await Task.Delay(1000);
            var st = await _engine.GetProgressAsync(jobId);
            if (st == null)
            {
                if (string.IsNullOrEmpty(_engine.LastError))
                    Error = "Brak odpowiedzi z silnika pobierania.";
                else
                    Error = _engine.LastError;
                return false;
            }

            Progress = (int)Math.Round(st.Progress);

            if (st.Status == "completed")
            {
                Status = $"Zakończono: {st.OutputPath}";
                return true;
            }
            if (st.Status == "failed")
            {
                Error = string.IsNullOrEmpty(st.Error) ? "Pobieranie nie powiodło się." : st.Error;
                return false;
            }

            Status = $"Pobieranie {marker}{Progress}%";
        }
    }

    public void ChooseFolder()
    {
        // Widok (MainWindow) obsługuje FolderBrowserDialog i ustawia OutputFolder.
    }

    public void ActivateLicense()
    {
        var key = _license.GenerateTestKey();
        _license.SaveLicense(key);
        IsPremium = true;
        BannerAd = null;
        SidebarAd = null;
        SidebarAd2 = null;
        InlineAd = null;
        InterstitialAd = null;
        Status = "Wersja premium aktywna – reklamy wyłączone.";
    }

    public event PropertyChangedEventHandler? PropertyChanged;

    protected void OnPropertyChanged([CallerMemberName] string? name = null)
        => PropertyChanged?.Invoke(this, new PropertyChangedEventArgs(name));
}
