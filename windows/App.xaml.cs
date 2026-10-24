using System.Windows;
using SocialDownloader.Services;
using SocialDownloader.ViewModels;

namespace SocialDownloader;

public partial class App : System.Windows.Application
{
    protected override void OnStartup(StartupEventArgs e)
    {
        base.OnStartup(e);

        var engine = new EngineClient();
        var license = new LicenseService();
        var ads = new AdService(engine);

        // Motyw: zapisany wybór w %APPDATA%\SocialDownloader\ui.json;
        // OPS jest premium-only — bez licencji wracamy na Trident (fail-closed).
        var theme = UiSettingsService.LoadTheme();
        if (ThemeService.IsPremiumTheme(theme) && !license.IsPremium)
        {
            theme = "trident";
        }
        ThemeService.Apply(theme);

        var viewModel = new MainViewModel(engine, license, ads);
        var mainWindow = new MainWindow
        {
            DataContext = viewModel
        };
        // Eventy podpinamy PO ustawieniu DataContext (ctor MainWindow robił to
        // wcześniej i ViewModel brał null — NullReferenceException przy starcie).
        mainWindow.HookViewModelEvents();
        mainWindow.Show();
    }
}
