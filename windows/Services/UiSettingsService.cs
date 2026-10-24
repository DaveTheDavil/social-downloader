using System;
using System.IO;
using System.Text.Json;

namespace SocialDownloader.Services;

/// <summary>
/// Ustawienia UI w %APPDATA%\SocialDownloader\ui.json (na razie tylko motyw).
/// </summary>
public static class UiSettingsService
{
    private static readonly string Dir =
        Path.Combine(Environment.GetFolderPath(Environment.SpecialFolder.ApplicationData), "SocialDownloader");
    private static readonly string FilePath = Path.Combine(Dir, "ui.json");

    public static string LoadTheme()
    {
        try
        {
            if (!File.Exists(FilePath)) return "trident";
            using var doc = JsonDocument.Parse(File.ReadAllText(FilePath));
            var theme = doc.RootElement.TryGetProperty("theme", out var t) ? t.GetString() : null;
            return string.IsNullOrWhiteSpace(theme) ? "trident" : theme!;
        }
        catch
        {
            // Pośledni JSON/BOM-śmieci — spokojnie wracamy do domyślnego motywu.
            return "trident";
        }
    }

    public static void SaveTheme(string theme)
    {
        try
        {
            Directory.CreateDirectory(Dir);
            var json = JsonSerializer.Serialize(new { theme }, new JsonSerializerOptions { WriteIndented = true });
            File.WriteAllText(FilePath, json);
        }
        catch
        {
            // Zapis preferencji kosmetycznej — braki nie mogą wybijać aplikacji.
        }
    }
}
