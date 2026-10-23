using System.Text.Json.Serialization;

namespace SocialDownloader.Models;

/// <summary>
/// Pojedyncza reklama z GET /ads (kontrakt: internal/ads/ads.go AdUnit).
/// </summary>
public class AdUnit
{
    [JsonPropertyName("id")]
    public string Id { get; set; } = string.Empty;
    [JsonPropertyName("advertiser")]
    public string Advertiser { get; set; } = string.Empty;
    [JsonPropertyName("title")]
    public string Title { get; set; } = string.Empty;
    [JsonPropertyName("description")]
    public string Description { get; set; } = string.Empty;
    [JsonPropertyName("url")]
    public string Url { get; set; } = string.Empty;
    [JsonPropertyName("image")]
    public string Image { get; set; } = string.Empty;
    [JsonPropertyName("type")]
    public string Type { get; set; } = string.Empty;
    [JsonPropertyName("weight")]
    public int Weight { get; set; }
    [JsonPropertyName("seconds")]
    public int Seconds { get; set; }
    [JsonPropertyName("clickthrough")]
    public string Clickthrough { get; set; } = string.Empty;
}

/// <summary>
/// Odpowiedź GET /ads: {"ad": …, "premium": bool}.
/// </summary>
public class AdResponse
{
    [JsonPropertyName("ad")]
    public AdUnit? Ad { get; set; }
    [JsonPropertyName("premium")]
    public bool Premium { get; set; }
}
