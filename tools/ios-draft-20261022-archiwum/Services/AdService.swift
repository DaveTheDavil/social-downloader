import Foundation

final class AdService {
    private let engine: EngineClient

    init(engine: EngineClient) {
        self.engine = engine
    }

    func getBannerAd() async -> AdUnit? {
        do {
            let resp = try await engine.getAd(type: "banner")
            if resp?.premium == true || resp?.ad == nil {
                return nil
            }
            return resp?.ad
        } catch {
            return nil
        }
    }

    func getInterstitialAd() async -> AdUnit? {
        do {
            let resp = try await engine.getAd(type: "interstitial")
            if resp?.premium == true || resp?.ad == nil {
                return nil
            }
            return resp?.ad
        } catch {
            return nil
        }
    }
}
