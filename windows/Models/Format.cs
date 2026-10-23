using System.Text.Json.Serialization;

namespace SocialDownloader.Models;

/// <summary>
/// Jeden format wideo/dźwięku z GET /info (kontrakt: internal/downloader/ydlp.go Format).
/// </summary>
public class Format
{
    [JsonPropertyName("format_id")]
    public string FormatId { get; set; } = string.Empty;
    [JsonPropertyName("ext")]
    public string Ext { get; set; } = string.Empty;
    [JsonPropertyName("height")]
    public int Height { get; set; }
    [JsonPropertyName("width")]
    public int Width { get; set; }
    [JsonPropertyName("filesize")]
    public long Filesize { get; set; }
    [JsonPropertyName("vcodec")]
    public string VCodec { get; set; } = string.Empty;
    [JsonPropertyName("acodec")]
    public string ACodec { get; set; } = string.Empty;
    [JsonPropertyName("format_note")]
    public string FormatNote { get; set; } = string.Empty;
}
