using System.Diagnostics;
using System.Windows;
using System.Windows.Threading;
using SocialDownloader.Models;

namespace SocialDownloader.Views;

public partial class AdInterstitial : Window
{
    private int _secondsLeft;
    private readonly DispatcherTimer _timer;

    public AdInterstitial(AdUnit ad)
    {
        InitializeComponent();
        DataContext = ad;
        _secondsLeft = ad.Seconds > 0 ? ad.Seconds : 5;
        SkipButton.Content = $"Pomiń ({_secondsLeft}s)";

        _timer = new DispatcherTimer { Interval = TimeSpan.FromSeconds(1) };
        _timer.Tick += Timer_Tick;
        _timer.Start();
    }

    private void Timer_Tick(object? sender, EventArgs e)
    {
        _secondsLeft--;
        SkipButton.Content = $"Pomiń ({_secondsLeft}s)";
        if (_secondsLeft <= 0)
        {
            _timer.Stop();
            SkipButton.IsEnabled = true;
            SkipButton.Content = "Pomiń";
        }
    }

    private void GoToAdvertiser_Click(object sender, RoutedEventArgs e)
    {
        if (DataContext is AdUnit ad && !string.IsNullOrEmpty(ad.Url))
        {
            try { Process.Start(new ProcessStartInfo { FileName = ad.Url, UseShellExecute = true }); }
            catch { }
        }
        DialogResult = true;
        Close();
    }

    private void Skip_Click(object sender, RoutedEventArgs e)
    {
        DialogResult = true;
        Close();
    }
}
