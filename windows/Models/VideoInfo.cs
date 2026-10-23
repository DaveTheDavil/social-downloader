using System.Text.Json.Serialization;

namespace SocialDownloader.Models;

/// <summary>
/// Odpowiedź GET /info silnika. Klucze JSON są snake_case (kontrakt w server.go),
/// atrybuty JsonPropertyName wymagane — System.Text.Json nie mapuje podkreślników.
/// </summary>
public class VideoInfo
{
    [JsonPropertyName("id")]
    public string Id { get; set; } = string.Empty;
    [JsonPropertyName("title")]
    public string Title { get; set; } = string.Empty;
    [JsonPropertyName("duration")]
    public int Duration { get; set; }
    [JsonPropertyName("uploader")]
    public string Uploader { get; set; } = string.Empty;
    [JsonPropertyName("formats")]
    public List<Format> Formats { get; set; } = new();
    [JsonPropertyName("qualities")]
    public List<int> Qualities { get; set; } = new();
    [JsonPropertyName("premium")]
    public bool Premium { get; set; }
    [JsonPropertyName("hwid")]
    public string Hwid { get; set; } = string.Empty;
    [JsonPropertyName("features")]
    public List<string> Features { get; set; } = new();
    [JsonPropertyName("is_playlist")]
    public bool IsPlaylist { get; set; }
    [JsonPropertyName("entries")]
    public List<EntryInfo> Entries { get; set; } = new();
    [JsonPropertyName("playlist_note")]
    public string PlaylistNote { get; set; } = string.Empty;
}

/// <summary>
/// Jedna pozycja publikacji z GET /info (seria IG lub pojedyncze wideo).
/// </summary>
public class EntryInfo
{
    [JsonPropertyName("index")]
    public int Index { get; set; }
    [JsonPropertyName("id")]
    public string Id { get; set; } = string.Empty;
    [JsonPropertyName("title")]
    public string Title { get; set; } = string.Empty;
    [JsonPropertyName("duration")]
    public int Duration { get; set; }
    [JsonPropertyName("is_image")]
    public bool IsImage { get; set; }
    [JsonPropertyName("qualities")]
    public List<int> Qualities { get; set; } = new();
    [JsonPropertyName("formats")]
    public List<Format> Formats { get; set; } = new();
}
