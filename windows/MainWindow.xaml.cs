using System;
using System.Windows;
using System.Windows.Forms;
using SocialDownloader.ViewModels;
using SocialDownloader.Views;

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

public sealed class BooleanToColorConverter : System.Windows.Data.IValueConverter
{
    public static readonly BooleanToColorConverter Instance = new();
    public object Convert(object value, Type targetType, object parameter, System.Globalization.CultureInfo culture)
        => value is bool b && b ? System.Windows.Media.Brushes.Green : System.Windows.Media.Brushes.Gray;
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

    private void CloseBanner_Click(object sender, RoutedEventArgs e)
    {
        AdBannerBorder.Visibility = Visibility.Collapsed;
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
}
