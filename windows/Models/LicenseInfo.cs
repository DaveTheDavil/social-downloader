using System.Text.Json.Serialization;

namespace SocialDownloader.Models;

/// <summary>
/// Odpowiedź GET /license: {"premium": bool, "hwid": string, "features": [...]}.
/// </summary>
public class LicenseInfo
{
    [JsonPropertyName("premium")]
    public bool Premium { get; set; }
    [JsonPropertyName("hwid")]
    public string Hwid { get; set; } = string.Empty;
    [JsonPropertyName("features")]
    public List<string> Features { get; set; } = new();
}
