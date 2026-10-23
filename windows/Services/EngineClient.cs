using System.Net.Http;
using System.Net.Http.Json;
using System.Text.Json.Serialization;
using SocialDownloader.Models;

namespace SocialDownloader.Services;

public class EngineClient : IDisposable
{
    private readonly HttpClient _http;
    private readonly string _baseUrl;
    private string _lastError = string.Empty;

    /// <summary>
    /// Ostatni przyjazny komunikat błędu komunikacji z silnikiem (np. brak połączenia).
    /// Czyszczony przy każdej udanej odpowiedzi HTTP.
    /// </summary>
    public string LastError
    {
        get => _lastError;
        private set
        {
            if (_lastError != value)
            {
                _lastError = value;
            }
        }
    }

    public EngineClient(string baseUrl = "http://127.0.0.1:8765")
    {
        _baseUrl = baseUrl;
        // /info może trwać dłużej (ekstrakcja yt-dlp w silniku ma deadline 3 min).
        _http = new HttpClient { Timeout = TimeSpan.FromSeconds(300) };
    }

    /// <summary>
    /// Czyta body błędu silnika ({"error": "..."} lub surowy tekst) na przyjazny komunikat.
    /// </summary>
    private static async Task<string?> ReadErrorBodyAsync(HttpResponseMessage? resp)
    {
        if (resp == null) return null;
        try
        {
            var text = await resp.Content.ReadAsStringAsync();
            try
            {
                var doc = System.Text.Json.JsonDocument.Parse(text);
                if (doc.RootElement.ValueKind == System.Text.Json.JsonValueKind.Object
                    && doc.RootElement.TryGetProperty("error", out var e)
                    && e.ValueKind == System.Text.Json.JsonValueKind.String)
                {
                    return e.GetString();
                }
            }
            catch { /* nie-JSON — spróbuj surowego tekstu */ }
            if (!string.IsNullOrWhiteSpace(text) && text.Length < 300)
                return text.Trim();
        }
        catch { }
        return null;
    }

    /// <summary>
    /// Wykonuje żądanie HTTP i tłumaczy błędy sieciowe na przyjazny komunikat.
    /// Zwraca null, gdy silnik jest niedostępny (brak połączenia, timeout, błąd HTTP).
    /// </summary>
    private async Task<HttpResponseMessage?> RequestAsync(Func<Task<HttpResponseMessage>> send)
    {
        try
        {
            var resp = await send();
            LastError = string.Empty;
            return resp;
        }
        catch (HttpRequestException)
        {
            LastError = "Brak połączenia z silnikiem pobierania (127.0.0.1:8765). Upewnij się, że serwer Go jest uruchomiony.";
            return null;
        }
        catch (TaskCanceledException)
        {
            LastError = "Silnik pobierania nie odpowiada (przekroczono czas oczekiwania).";
            return null;
        }
        catch (Exception ex)
        {
            LastError = $"Błąd silnika pobierania: {ex.Message}";
            return null;
        }
    }

    public async Task<VideoInfo?> GetInfoAsync(string url)
    {
        var resp = await RequestAsync(() => _http.GetAsync($"{_baseUrl}/info?url={Uri.EscapeDataString(url)}"));
        if (resp == null) return null;
        if (!resp.IsSuccessStatusCode)
        {
            LastError = (await ReadErrorBodyAsync(resp)) ?? $"Silnik zwrócił HTTP {(int)resp.StatusCode} przy analizie linku.";
            return null;
        }
        return await resp.Content.ReadFromJsonAsync<VideoInfo>();
    }

    public async Task<AdResponse?> GetAdAsync(string type)
    {
        var resp = await RequestAsync(() => _http.GetAsync($"{_baseUrl}/ads?type={type}"));
        if (resp == null) return null;
        if (!resp.IsSuccessStatusCode)
        {
            LastError = (await ReadErrorBodyAsync(resp)) ?? $"Silnik zwrócił HTTP {(int)resp.StatusCode} przy reklamie.";
            return null;
        }
        return await resp.Content.ReadFromJsonAsync<AdResponse>();
    }

    public async Task<LicenseInfo?> GetLicenseAsync()
    {
        var resp = await RequestAsync(() => _http.GetAsync($"{_baseUrl}/license"));
        if (resp == null) return null;
        if (!resp.IsSuccessStatusCode)
        {
            LastError = (await ReadErrorBodyAsync(resp)) ?? $"Silnik zwrócił HTTP {(int)resp.StatusCode} przy licencji.";
            return null;
        }
        return await resp.Content.ReadFromJsonAsync<LicenseInfo>();
    }

    public async Task<string?> StartDownloadAsync(string url, string formatId, string outputPath, int item = 1)
    {
        var body = new { url, format_id = formatId, output_path = outputPath, item };
        var resp = await RequestAsync(() => _http.PostAsJsonAsync($"{_baseUrl}/download", body));
        if (resp == null) return null;
        if (!resp.IsSuccessStatusCode)
        {
            LastError = (await ReadErrorBodyAsync(resp)) ?? $"Silnik zwrócił HTTP {(int)resp.StatusCode} przy pobieraniu.";
            return null;
        }
        var doc = await resp.Content.ReadFromJsonAsync<DownloadResponse>();
        return doc?.JobId;
    }

    /// <summary>
    /// GET /ig-cookie → stan sesji Instagram (true/false; null gdy silnik nieosiągalny).
    /// </summary>
    public async Task<bool?> GetIgCookieConfiguredAsync()
    {
        var resp = await RequestAsync(() => _http.GetAsync($"{_baseUrl}/ig-cookie"));
        if (resp == null) return null;
        if (!resp.IsSuccessStatusCode)
        {
            LastError = (await ReadErrorBodyAsync(resp)) ?? $"Silnik zwrócił HTTP {(int)resp.StatusCode} przy sesji IG.";
            return null;
        }
        var doc = await resp.Content.ReadFromJsonAsync<IgCookieResponse>();
        return doc?.Configured;
    }

    /// <summary>
    /// POST /ig-cookie: zapis/czyszczenie sesji Instagram (cookie = "" → kasuje).
    /// Zwraca configured z odpowiedzi silnika (false przy błędzie).
    /// </summary>
    public async Task<bool> SetIgCookieAsync(string cookie)
    {
        var resp = await RequestAsync(() => _http.PostAsJsonAsync($"{_baseUrl}/ig-cookie", new { cookie }));
        if (resp == null) return false;
        if (!resp.IsSuccessStatusCode)
        {
            LastError = (await ReadErrorBodyAsync(resp)) ?? $"Silnik zwrócił HTTP {(int)resp.StatusCode} przy zapisie sesji IG.";
            return false;
        }
        var doc = await resp.Content.ReadFromJsonAsync<IgCookieResponse>();
        return doc?.Configured ?? false;
    }

    public async Task<bool> HealthCheckAsync()
    {
        try
        {
            var resp = await _http.GetAsync($"{_baseUrl}/healthz");
            LastError = string.Empty;
            return resp.IsSuccessStatusCode;
        }
        catch (HttpRequestException)
        {
            LastError = "Brak połączenia z silnikiem pobierania (127.0.0.1:8765). Upewnij się, że serwer Go jest uruchomiony.";
            return false;
        }
        catch (TaskCanceledException)
        {
            LastError = "Silnik pobierania nie odpowiada (przekroczono czas oczekiwania).";
            return false;
        }
        catch (Exception ex)
        {
            LastError = $"Błąd silnika pobierania: {ex.Message}";
            return false;
        }
    }

    public async Task<DownloadStatus?> GetProgressAsync(string jobId)
    {
        var resp = await RequestAsync(() => _http.GetAsync($"{_baseUrl}/progress?job_id={Uri.EscapeDataString(jobId)}"));
        if (resp == null) return null;
        if (!resp.IsSuccessStatusCode)
        {
            LastError = (await ReadErrorBodyAsync(resp)) ?? $"Silnik zwrócił HTTP {(int)resp.StatusCode} przy postępie.";
            return null;
        }
        return await resp.Content.ReadFromJsonAsync<DownloadStatus>();
    }

    public void Dispose() => _http.Dispose();
}

public class DownloadStatus
{
    [JsonPropertyName("job_id")]
    public string JobId { get; set; } = string.Empty;
    [JsonPropertyName("status")]
    public string Status { get; set; } = string.Empty;
    [JsonPropertyName("progress")]
    public double Progress { get; set; }
    [JsonPropertyName("error")]
    public string Error { get; set; } = string.Empty;
    [JsonPropertyName("output_path")]
    public string OutputPath { get; set; } = string.Empty;
    [JsonPropertyName("url")]
    public string Url { get; set; } = string.Empty;
}

public class DownloadResponse
{
    [JsonPropertyName("job_id")]
    public string JobId { get; set; } = string.Empty;
    [JsonPropertyName("status")]
    public string Status { get; set; } = string.Empty;
}

public class IgCookieResponse
{
    [JsonPropertyName("ok")]
    public bool Ok { get; set; }
    [JsonPropertyName("configured")]
    public bool Configured { get; set; }
}
