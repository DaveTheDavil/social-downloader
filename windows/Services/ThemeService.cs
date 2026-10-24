using System;
using System.Collections.Generic;
using System.Windows;

namespace SocialDownloader.Services;

/// <summary>
/// Przełączanie motywów (DynamicResource nad App.Resources).
/// Kanoniczne motywy: trident (główny, ciemny), plakat (jasny) — dla wszystkich;
/// ops — wyłącznie premium ("premka odblokowuje OPS"), egzekwowane na dwóch bramkach:
/// przed OnStartup (fallback) i przy wyborze w combo (MainWindow).
/// </summary>
public static class ThemeService
{
    public static string Current { get; private set; } = "trident";

    public static IReadOnlyList<string> Names { get; } = new[] { "trident", "plakat", "ops" };

    public static bool IsPremiumTheme(string name)
        => string.Equals(name, "ops", StringComparison.OrdinalIgnoreCase);

    public static string DisplayName(string name) => name?.ToLowerInvariant() switch
    {
        "trident" => "Trident (ciemny)",
        "plakat" => "Plakat (jasny)",
        "ops" => "OPS (premium)",
        _ => name ?? "Trident",
    };

    /// <summary>Aplikuje motyw; niezależnie od wejścia daje poprawny słownik (fail-closed → trident).</summary>
    public static void Apply(string name)
    {
        // normalizacja: tylko znane słowniki Theme-<Cap>.xaml w pakiecie
        var key = (name ?? string.Empty).Trim().ToLowerInvariant() switch
        {
            "plakat" => "Plakat",
            "ops" => "Ops",
            _ => "Trident",
        };

        var dict = new ResourceDictionary
        {
            Source = new Uri($"pack://application:,,,/Themes/Theme-{key}.xaml")
        };

        var app = System.Windows.Application.Current;
        var merged = app.Resources.MergedDictionaries;
        if (merged.Count == 0)
        {
            merged.Add(dict);
        }
        else
        {
            // [0] = Theme-*.xaml, [1] = Shared.xaml (App.xaml) — podmieniamy tylko theme
            merged[0] = dict;
        }

        Current = key.ToLowerInvariant();
    }
}
