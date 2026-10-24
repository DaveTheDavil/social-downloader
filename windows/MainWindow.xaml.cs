using System;
using System.Diagnostics;
using System.Linq;
using System.Windows;
using System.Windows.Controls;
using System.Windows.Forms;
using System.Windows.Input;
using System.Windows.Media;
using System.Windows.Media.Imaging;
using SocialDownloader.Models;
using SocialDownloader.Services;
using SocialDownloader.ViewModels;
using SocialDownloader.Views;

// UseWindowsForms + UseWPF importują globalnie System.Windows.Forms/Application
// stąd jawne aliasy, żeby Application/Brush/Brushes wskazywały WPF-a
using Application = System.Windows.Application;
using Brush = System.Windows.Media.Brush;
using Brushes = System.Windows.Media.Brushes;

namespace SocialDownloader;

public sealed class BooleanToVisibilityConverter : System.Windows.Data.IValueConverter
{
    public static readonly BooleanToVisibilityConverter Instance = new();
    public object Convert(object value, Type targetType, object parameter, System.Globalization.CultureInfo culture)
        => value is bool b && b ? Visibility.Visible : Visibility.Collapsed;
    public object ConvertBack(object value, Type targetType, object parameter, System.Globalization.CultureInfo culture)
        => throw new NotImplementedException();
}

/// <summary>
/// true → ukryte, false → widoczne. Do ukrywania panelu aktywacji premium,
/// gdy licencja jest już aktywna (IsPremium == true).
/// </summary>
public sealed class BooleanToVisibilityInverseConverter : System.Windows.Data.IValueConverter
{
    public static readonly BooleanToVisibilityInverseConverter Instance = new();
    public object Convert(object value, Type targetType, object parameter, System.Globalization.CultureInfo culture)
        => value is bool b && b ? Visibility.Collapsed : Visibility.Visible;
    public object ConvertBack(object value, Type targetType, object parameter, System.Globalization.CultureInfo culture)
        => throw new NotImplementedException();
}

/// <summary>Tekst "Premium: Tak/Nie" koloruje z palety aktywnego motywu (Brush.Ok / Brush.TextMuted).</summary>
public sealed class BooleanToColorConverter : System.Windows.Data.IValueConverter
{
    public static readonly BooleanToColorConverter Instance = new();
    public object Convert(object value, Type targetType, object parameter, System.Globalization.CultureInfo culture)
    {
        var key = value is bool b && b ? "Brush.Ok" : "Brush.TextMuted";
        if (Application.Current?.TryFindResource(key) is Brush themed) return themed;
        return value is bool ok && ok ? Brushes.Green : Brushes.Gray;
    }
    public object ConvertBack(object value, Type targetType, object parameter, System.Globalization.CultureInfo culture)
        => throw new NotImplementedException();
}

public partial class MainWindow : Window
{
    private MainViewModel ViewModel => (MainViewModel)DataContext;

    public MainWindow()
    {
        InitializeComponent();
    }

    /// <summary>
    /// Podpinanie eventów ViewModelu do View — po ustawieniu DataContext
    /// (w ctorze DataContext jest jeszcze null; stąd wkrótce NRE).
    /// </summary>
    public void HookViewModelEvents()
    {
        ViewModel.InterstitialRequested += ViewModel_InterstitialRequested;
        ViewModel.DownloadCompleted += ViewModel_DownloadCompleted;
        ViewModel.PropertyChanged += ViewModel_PropertyChanged;
        InitThemeMenu();
    }

    private void ViewModel_InterstitialRequested(object? sender, EventArgs e)
    {
        var ad = ViewModel.InterstitialAd;
        if (ad == null) return;

        var dlg = new AdInterstitial(ad) { Owner = this };
        dlg.ShowDialog();
        // Po zamknięciu okna interstitial (pominięciu lub po odliczaniu) startujemy pobieranie.
        ViewModel.OnInterstitialClosed();
    }

    private void ViewModel_DownloadCompleted(object? sender, EventArgs e)
    {
        System.Windows.MessageBox.Show("Pobieranie zakończone pomyślnie!", "Social Downloader",
            System.Windows.MessageBoxButton.OK, System.Windows.MessageBoxImage.Information);
    }

    private void ViewModel_PropertyChanged(object? sender, System.ComponentModel.PropertyChangedEventArgs e)
    {
        if (e.PropertyName == nameof(ViewModel.IsPremium))
        {
            // Premka odblokowuje motyw OPS — przebuduj menu motywów (item przestaje być disabled).
            InitThemeMenu();
        }
    }

    private void CloseLeaderboard_Click(object sender, RoutedEventArgs e)
    {
        LeaderboardAdBorder.Visibility = Visibility.Collapsed;
    }

    private void CloseSidebar_Click(object sender, RoutedEventArgs e)
    {
        SidebarAdBorder.Visibility = Visibility.Collapsed;
    }

    private void CloseSidebar2_Click(object sender, RoutedEventArgs e)
    {
        SidebarAd2Border.Visibility = Visibility.Collapsed;
    }

    private void CloseInline_Click(object sender, RoutedEventArgs e)
    {
        InlineAdBorder.Visibility = Visibility.Collapsed;
    }

    /// <summary>Klik na slot reklamowy → otwarcie linku reklamodawcy (demo-kreatywy nie mają URL).</summary>
    private void OpenAdSlot_Click(object sender, MouseButtonEventArgs e)
    {
        if ((sender as FrameworkElement)?.Tag is not AdUnit ad) return;
        if (string.IsNullOrWhiteSpace(ad.Url)) return;
        try
        {
            Process.Start(new ProcessStartInfo { FileName = ad.Url, UseShellExecute = true });
        }
        catch
        {
            // URL złośliwy/brak handlera — cicho ignorujemy (slot pozostaje).
        }
    }
    private void ChooseFolder_Click(object sender, RoutedEventArgs e)
    {
        var dlg = new FolderBrowserDialog
        {
            Description = "Wybierz folder docelowy dla pobranych plików",
            ShowNewFolderButton = true
        };
        if (dlg.ShowDialog() == System.Windows.Forms.DialogResult.OK)
        {
            ViewModel.OutputFolder = dlg.SelectedPath;
        }
    }

    // ══════════════════ MOTYWY (ThemeService + UiSettingsService) ══════════════════

    private void InitThemeMenu()
    {
        var selected = ThemeCombo.SelectedItem is ComboBoxItem cur ? cur.Tag as string : null;

        ThemeCombo.Items.Clear();
        foreach (var name in ThemeService.Names)
        {
            var item = new ComboBoxItem { Tag = name, Content = ThemeService.DisplayName(name) };
            if (ThemeService.IsPremiumTheme(name) && !ViewModel.IsPremium)
            {
                item.IsEnabled = false;
                item.Content = ThemeService.DisplayName(name) + " — wymaga premium";
            }
            ThemeCombo.Items.Add(item);
        }

        // zachowaj dotychczasowy wybór (np. po odblokowaniu premki), w innym razie → aktywny motyw
        var target = FindItemByTag(selected) ?? FindItemByTag(ThemeService.Current);
        ThemeCombo.SelectedItem = target; // programowe ustawienie — SelectionChanged zejdzie po no-op check
        RefreshHeaderLogo();
    }

    private ComboBoxItem? FindItemByTag(string? tag)
        => ThemeCombo.Items.OfType<ComboBoxItem>().FirstOrDefault(i => string.Equals(i.Tag as string, tag, StringComparison.OrdinalIgnoreCase));

    private void ThemeCombo_SelectionChanged(object sender, SelectionChangedEventArgs e)
    {
        if (e.AddedItems.OfType<ComboBoxItem>().FirstOrDefault() is not { Tag: string name })
            return;
        if (string.Equals(name, ThemeService.Current, StringComparison.OrdinalIgnoreCase))
            return; // programowe ustawienie / init — nic nie rób

        if (ThemeService.IsPremiumTheme(name) && !ViewModel.IsPremium)
        {
            ShowThemeInfo("Motyw OPS odblokowuje wersja premium.");
            RevertThemeSelection();
            return;
        }

        ThemeService.Apply(name);
        UiSettingsService.SaveTheme(name);
        RefreshHeaderLogo();
    }

    private void RevertThemeSelection()
    {
        var active = FindItemByTag(ThemeService.Current);
        if (active != null)
            ThemeCombo.SelectedItem = active; // wróci po no-op check w SelectionChanged
    }

    private void ShowThemeInfo(string text)
    {
        System.Windows.MessageBox.Show(text, "Social Downloader",
            System.Windows.MessageBoxButton.OK, System.Windows.MessageBoxImage.Information);
    }

    /// <summary>Nagłówek + brandowy baner: DUO na ciemnych motywach, RED na jasnym Plakacie (srebro na kremie ginie).</summary>
    private void RefreshHeaderLogo()
    {
        var variant = string.Equals(ThemeService.Current, "plakat", StringComparison.OrdinalIgnoreCase)
            ? "red"
            : "duo";
        var uri = new Uri($"pack://application:,,,/Assets/Logo/dtd-beast-trident-{variant}-48.png");
        HeaderLogo.Source = new BitmapImage(uri);
        if (BrandLogo != null)
        {
            BrandLogo.Source = new BitmapImage(uri);
        }
    }

    private const string PortalUrl = "https://thedavil.idsl.pl";

    private void OpenBrandPortal_Click(object sender, MouseButtonEventArgs e)
    {
        try
        {
            Process.Start(new ProcessStartInfo { FileName = PortalUrl, UseShellExecute = true });
        }
        catch
        {
            // Brak przeglądarki/handlera — baner zostaje, nic nie wybucha.
        }
    }
}
