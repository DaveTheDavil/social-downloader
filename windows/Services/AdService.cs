using SocialDownloader.Models;

namespace SocialDownloader.Services;

public class AdService
{
    private readonly EngineClient _engine;

    public AdService(EngineClient engine)
    {
        _engine = engine;
    }

    public async Task<AdUnit?> GetBannerAdAsync()
    {
        var resp = await _engine.GetAdAsync("banner");
        if (resp == null || resp.Premium || resp.Ad == null)
            return null;
        return resp.Ad;
    }

    public async Task<AdUnit?> GetInterstitialAdAsync()
    {
        var resp = await _engine.GetAdAsync("interstitial");
        if (resp == null || resp.Premium || resp.Ad == null)
            return null;
        return resp.Ad;
    }
}
