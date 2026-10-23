import SwiftUI
import Combine

final class MainViewModel: ObservableObject {
    @Published var url: String = ""
    @Published var videoInfo: VideoInfo?
    @Published var selectedFormat: Format?
    @Published var isDownloading: Bool = false
    @Published var progress: Int = 0
    @Published var status: String = "Wklej link do filmu/zdjęcia z FB, IG, TT lub YT."
    @Published var error: String = ""
    @Published var bannerAd: AdUnit?
    @Published var interstitialAd: AdUnit?
    @Published var isPremium: Bool = false
    @Published var outputFolder: String = ""

    var showAds: Bool { !isPremium }

    var onInterstitialRequested: (() -> Void)?
    var onDownloadCompleted: (() -> Void)?

    private let engine: EngineClient
    private let license: LicenseService
    private let ads: AdService
    private var cancellables = Set<AnyCancellable>()
    private var currentJobId: String? = nil
    private var progressTask: Task<Void, Never>?

    init(engine: EngineClient, license: LicenseService, ads: AdService) {
        self.engine = engine
        self.license = license
        self.ads = ads
        self.isPremium = license.isPremium
        self.outputFolder = defaultOutputFolder()
    }

    private func defaultOutputFolder() -> String {
        FileManager.default.urls(for: .documentDirectory, in: .userDomainMask).first?.path ?? ""
    }

    @MainActor
    func analyze() async {
        error = ""
        status = "Analizowanie linku..."
        isDownloading = true

        defer { isDownloading = false }

        do {
            let info = try await engine.getInfo(url: url)
            videoInfo = info
            isPremium = info.premium

            if isPremium {
                status = "Wersja premium aktywna – brak reklam, pełna rozdzielczość."
            } else {
                status = "Tryb darmowy – dostępne do 720p. Reklamy są wyświetlane."
            }

            if !isPremium {
                await loadBannerAd()
            }
        } catch {
            self.error = errorMessage(for: error)
            status = "Błąd analizy."
        }
    }

    @MainActor
    private func loadBannerAd() async {
        bannerAd = await ads.getBannerAd()
    }

    @MainActor
    func download() async {
        guard let format = selectedFormat, let info = videoInfo else { return }

        if info.duration > 600 && (url.contains("youtube.com") || url.contains("youtu.be")) {
            error = "YouTube: filmy powyżej 10 minut są zablokowane (unikanie longplayów)."
            return
        }

        if !isPremium && format.height > 720 {
            error = "Rozdzielczość powyżej 720p wymaga wersji premium."
            return
        }

        if !isPremium {
            interstitialAd = await ads.getInterstitialAd()
            if interstitialAd != nil {
                onInterstitialRequested?()
                return
            }
        }

        await startDownload()
    }

    @MainActor
    func onInterstitialClosed() {
        interstitialAd = nil
        Task { await startDownload() }
    }

    @MainActor
    private func startDownload() async {
        guard let format = selectedFormat, let info = videoInfo else { return }

        let fileName = "\(info.title).\(format.ext)"
            .replacingOccurrences(of: ":", with: "_")
            .replacingOccurrences(of: "/", with: "_")
            .replacingOccurrences(of: "\\", with: "_")
        let outputPath = outputFolder + "/" + fileName

        status = "Pobieranie..."
        progress = 0
        error = ""
        isDownloading = true

        do {
            guard let jobId = try await engine.startDownload(url: url, formatId: format.formatId, outputPath: outputPath) else {
                self.error = "Serwer nie zwrócił identyfikatora zadania."
                status = "Błąd pobierania."
                isDownloading = false
                return
            }
            currentJobId = jobId
            progressTask = Task.detached {
                await self.pollProgress()
            }
            status = "Pobieranie trwa w tle..."
        } catch {
            self.error = errorMessage(for: error)
            status = "Błąd pobierania."
            isDownloading = false
        }
    }

    private func pollProgress() async {
        guard let currentJobId = currentJobId else { return }

        while !Task.isCancelled {
            do {
                try await Task.sleep(nanoseconds: 1_000_000_000) // 1 sekunda
                guard !Task.isCancelled else { break }
                let status = try await engine.getProgress(jobId: currentJobId)
                if let s = status {
                    await MainActor.run {
                        self.progress = Int(round(s.progress))
                        if s.status == "completed" {
                            self.status = "Zakończono: \(s.outputPath ?? "")"
                            self.isDownloading = false
                            self.onDownloadCompleted?()
                            progressTask?.cancel()
                        } else if s.status == "failed" {
                            self.error = s.error ?? "Pobieranie nie powiodło się."
                            self.status = "Błąd."
                            self.isDownloading = false
                            progressTask?.cancel()
                        } else {
                            self.status = "Pobieranie... \(self.progress)%"
                        }
                    }
                }
            } catch {
                await MainActor.run {
                    self.error = errorMessage(for: error)
                    self.status = "Brak połączenia z silnikiem."
                    self.isDownloading = false
                }
                break
            }
        }
    }

    /// Przekształca błędy sieciowe na przyjazne komunikaty po polsku.
    private func errorMessage(for error: Error) -> String {
        if let urlError = error as? URLError {
            switch urlError.code {
            case .notConnectedToInternet:
                return "Brak połączenia z internetem."
            case .cannotConnectToHost, .networkConnectionLost, .cannotConnectToNetwork:
                return "Nie można połączyć się z silnikiem pobierania. Uruchom serwer Go i sprawdź adres."
            case .timedOut:
                return "Przekroczono czas oczekiwania na odpowiedź serwera."
            case .badURL:
                return "Nieprawidłowy adres serwera."
            case .badServerResponse:
                return "Serwer zwrócił nieprawidłową odpowiedź."
            default:
                return "Błąd sieci: \(urlError.localizedDescription)"
            }
        }
        if error is DecodingError {
            return "Nie można przetworzyć odpowiedzi serwera."
        }
        return error.localizedDescription
    }

    func activateLicense() {
        let key = license.generateTestKey()
        license.saveLicense(key)
        isPremium = true
        bannerAd = nil
        interstitialAd = nil
        status = "Wersja premium aktywna – reklamy wyłączone."
    }
}
