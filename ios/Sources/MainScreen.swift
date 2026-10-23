import SwiftUI

// Zakładka „Pobieraj": analiza linku, karta wyniku, wybór jakości, lista
// pozycji (seria), pobieranie sekwencyjne (POST /download → poll /progress →
// GET /file → zapis do Documents), baner + interstitial dla darmowych,
// panel sesji Instagram na dole (jak w kliencie Windows).
struct MainScreen: View {

    @EnvironmentObject private var store: AppStore

    // pole URL + analiza
    @State private var linkText = ""
    @State private var analyzing = false
    @State private var analysis: InfoResponse?
    @State private var analysisError = ""

    // wybór jakości + pozycji
    @State private var chosenQuality: Int?
    @State private var selected: Set<Int> = []

    // pobieranie (batch)
    @State private var batchRunning = false
    @State private var statusText = ""
    @State private var progressValue = 0.0

    // reklamy (darmowe: silnik zwraca ad != null; premium → null)
    @State private var bannerAd: AdUnit?
    @State private var interstitialAd: AdUnit?
    @State private var startAfterInterstitial = false

    // sesja Instagram (panel na dole, jak w kliencie Windows)
    @State private var igCookieText = ""
    @State private var igStatus = "Sprawdzam sesję…"
    @State private var igWorking = false

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 14) {
                Text("Uwaga: Aplikacja służy do pobierania wyłącznie treści, na które masz prawa lub które są w domenie publicznej. Użytkownik ponosi pełną odpowiedzialność za przestrzeganie regulaminów platform i praw autorskich.")
                    .font(.system(size: 11))
                    .foregroundColor(.gray)
                    .fixedSize(horizontal: false, vertical: true)

                if let ad = bannerAd {
                    BannerAdView(ad: ad)
                }

                linkSection
                resultSection

                if !statusText.isEmpty {
                    progressSection
                }

                if !analysisError.isEmpty {
                    analysisErrorSection
                }

                if let info = analysis {
                    qualitySection(info)
                    entriesSection(info)
                    if !info.playlistNote.isEmpty {
                        Text(info.playlistNote)
                            .font(.system(size: 11))
                            .foregroundColor(.orange)
                            .fixedSize(horizontal: false, vertical: true)
                    }
                    downloadButtonSection
                }

                igSessionSection
            }
            .padding()
        }
        .navigationTitle("Social Downloader")
        .task { await loadOnAppear() }
        .sheet(item: $interstitialAd) { ad in
            InterstitialView(ad: ad) {
                    // sheet zamknięty (odliczenie + Pomiń) → ruszamy z batchem
                Task { @MainActor in
                    if startAfterInterstitial {
                        startAfterInterstitial = false
                        await runBatch()
                    }
                }
            }
            .presentationDetents([.height(420)])
            .interactiveDismissDisabled(true)
        }
    }

    // MARK: pola i przycisk URL

    private var linkSection: some View {
        VStack(alignment: .leading, spacing: 8) {
            TextField("Wklej link (YouTube / Instagram / ...)", text: $linkText)
                .keyboardType(.URL)
                .autocorrectionDisabled()
                .textInputAutocapitalization(.never)
                .submitLabel(.go)
                .padding(10)
                .background(Color(.secondarySystemBackground))
                .cornerRadius(10)
            Button {
                Task { @MainActor in await analyze() }
            } label: {
                if analyzing {
                    ProgressView().frame(maxWidth: .infinity)
                } else {
                    Text("Analizuj").frame(maxWidth: .infinity)
                }
            }
            .buttonStyle(.borderedProminent)
            .disabled(analyzing || linkText.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty)
        }
    }

    // MARK: karta wyniku

    private var resultSection: some View {
        Group {
            if let info = analysis {
                VStack(alignment: .leading, spacing: 6) {
                    Text(info.title.isEmpty ? "(bez tytułu)" : info.title)
                        .font(.headline)
                    if info.duration > 0 {
                        Text("Czas: \(info.duration) s")
                    }
                    Text("Autor: \(info.uploader.isEmpty ? "?" : info.uploader)")
                    HStack(spacing: 6) {
                        Text("Premium:")
                        Text(info.premium ? "Tak" : "Nie")
                            .foregroundColor(info.premium ? .green : .gray)
                            .bold()
                    }
                }
                .font(.subheadline)
                .padding(12)
                .frame(maxWidth: .infinity, alignment: .leading)
                .background(Color(.secondarySystemBackground))
                .cornerRadius(10)
            }
        }
    }

    private var analysisErrorSection: some View {
        Text(analysisError)
            .font(.footnote)
            .foregroundColor(.red)
            .padding(10)
            .frame(maxWidth: .infinity, alignment: .leading)
            .background(Color(.secondarySystemBackground))
            .cornerRadius(10)
    }

    // MARK: jakość

    private func qualitySection(_ info: InfoResponse) -> some View {
        Group {
            if !info.qualities.isEmpty {
                HStack {
                    Text("Wybierz jakość:")
                        .font(.subheadline)
                    Spacer()
                    Picker("Jakość", selection: $chosenQuality) {
                        ForEach(info.qualities, id: \.self) { q in
                            Text("\(q)p").tag(Optional(q))
                        }
                    }
                    .pickerStyle(.menu)
                }
            }
        }
    }

    // MARK: lista pozycji

    private func entriesSection(_ info: InfoResponse) -> some View {
        Group {
            if info.entries.count > 1 {
                ForEach(info.entries, id: \.index) { entry in
                    Toggle(isOn: toggleBinding(entry)) {
                        entryRow(entry)
                    }
                    .padding(.vertical, 2)
                }
            } else if let only = info.entries.first {
                entryRow(only)  // pojedyncza publikacja = 1 pozycja, bez wyklików
            }
        }
    }

    private func entryRow(_ entry: Entry) -> some View {
        VStack(alignment: .leading, spacing: 2) {
            HStack {
                Text("\(entry.index). ")
                Text(entry.title.isEmpty ? "(bez tytułu)" : entry.title)
                    .bold()
            }
            HStack(spacing: 10) {
                Text(entryLabel(entry))
                    .foregroundColor(.secondary)
                if entry.duration > 0 {
                    Text("\(entry.duration) s")
                        .foregroundColor(.secondary)
                }
                if !entry.qualities.isEmpty {
                    Text("max \(entry.qualities.max() ?? 0)p")
                        .foregroundColor(.secondary)
                }
            }
            .font(.caption)
        }
    }

    private func entryLabel(_ entry: Entry) -> String {
        if entry.isImage { return "obraz (seria IG)" }
        if entry.formats.contains(where: { $0.formatId == "vid" }) { return "wideo (seria IG)" }
        return entry.formats.contains { !$0.isImageFormat } ? "wideo" : "plik"
    }

    private func toggleBinding(_ entry: Entry) -> Binding<Bool> {
        Binding<Bool>(
            get: { selected.contains(entry.index) },
            set: { on in
                if on {
                    selected.insert(entry.index)
                } else {
                    selected.remove(entry.index)
                }
            }
        )
    }

    // MARK: przycisk pobierania

    private var downloadButtonSection: some View {
        let picked = selected.count
        let isSeries = (analysis?.entries.count ?? 0) > 1
        return VStack(spacing: 8) {
            Button {
                Task { @MainActor in await beforeDownload() }
            } label: {
                Text(isSeries ? "Pobierz zaznaczone (\(picked))" : "Pobierz plik")
                    .frame(maxWidth: .infinity)
            }
            .buttonStyle(.borderedProminent)
            .disabled(picked == 0 || batchRunning || analyzing)
            if batchRunning {
                VStack(alignment: .leading, spacing: 4) {
                    ProgressView(value: min(max(progressValue, 0), 1))
                    Text(statusText).font(.footnote).foregroundColor(.secondary)
                }
            }
        }
    }

    private var progressSection: some View {
        VStack(alignment: .leading, spacing: 4) {
            ProgressView(value: min(max(progressValue, 0), 1))
            Text(statusText).font(.footnote).foregroundColor(.secondary)
        }
    }

    // MARK: sesja Instagram (panel dołkowy)

    private var igSessionSection: some View {
        VStack(alignment: .leading, spacing: 8) {
            HStack {
                Text("Sesja Instagram (pełne serie)")
                    .font(.subheadline)
                    .bold()
                Spacer()
                Text(igStatus).font(.caption).foregroundColor(.secondary)
            }
            TextField("Wklej ciastko sesji (cookie)", text: $igCookieText)
                .autocorrectionDisabled()
                .textInputAutocapitalization(.never)
                .font(.caption)
                .padding(8)
                .background(Color(.secondarySystemBackground))
                .cornerRadius(8)
            HStack(spacing: 10) {
                Button("Zapisz sesję") {
                    Task { @MainActor in await saveIgCookie("save") }
                }
                .buttonStyle(.bordered)
                .disabled(igWorking)
                Button("Wyczyść", role: .destructive) {
                    Task { @MainActor in await saveIgCookie("clear") }
                }
                .buttonStyle(.bordered)
                .disabled(igWorking)
            }
            .font(.caption)
        }
        .padding(.top, 6)
    }

    // MARK: akcje async

    private func loadOnAppear() async {
        await refreshIgStatus()
        await fetchBanner()
    }

    private func refreshIgStatus() async {
        do {
            let base = try store.effectiveEngineURL()
            let resp = try await EngineAPI.igCookieState(base: base)
            igStatus = resp.configured == true ? "sesja zapisana" : "brak sesji"
        } catch {
            igStatus = "?"
        }
    }

    // Darmowe: silnik zwraca baner (premium → ad=null → brak banera). Błąd
    // sieci przy /ads po prostu nie pokazuje banera.
    private func fetchBanner() async {
        guard let base = try? store.effectiveEngineURL() else { return }
        bannerAd = try? await EngineAPI.ads(base: base, type: "banner").ad
    }

    private func saveIgCookie(_ mode: String) async {
        igWorking = true
        defer { igWorking = false }
        do {
            let base = try store.effectiveEngineURL()
            let cookie = mode == "save" ? igCookieText.trimmingCharacters(in: .whitespacesAndNewlines) : ""
            _ = try await EngineAPI.saveIgCookie(base: base, cookie: cookie)
            igCookieText = ""
            await refreshIgStatus()
            igStatus = mode == "save" ? "sesja zapisana" : "sesja wyczyszczona"
        } catch {
            igStatus = "błąd: \(error.localizedDescription)"
        }
    }

    private func analyze() async {
        guard !analyzing else { return }
        analyzing = true
        analysisError = ""
        defer {
            analyzing = false
        }
        do {
            let base = try store.effectiveEngineURL()
            let info = try await EngineAPI.info(base: base, contentURL: linkText.trimmingCharacters(in: .whitespacesAndNewlines))
            analysis = info
            chosenQuality = info.qualities.max()
            selected = Set(info.entries.map(\.index)) // domyślnie zaznacz wszystko
        } catch {
            analysis = nil
            analysisError = error.localizedDescription
        }
    }

    // Pobranie (darmowe): najpierw interstitial z odliczaniem; po nim batch.
    private func beforeDownload() async {
        guard !batchRunning else { return }
        do {
            let base = try store.effectiveEngineURL()
            let resp = try await EngineAPI.ads(base: base, type: "interstitial")
            if let ad = resp.ad {
                startAfterInterstitial = true
                interstitialAd = ad  // sheet; runBatch() rusza z onDone
                return
            }
        } catch {
            // bez reklamy po prostu pobieramy
        }
        await runBatch()
    }

    // MARK: batch — sekwencyjnie, jak w kliencie Windows

    private func runBatch() async {
        guard let info = analysis, !batchRunning else { return }
        let picks = info
            .entries
            .filter { selected.contains($0.index) }
            .sorted { $0.index < $1.index }
        guard !picks.isEmpty else { return }

        batchRunning = true
        progressValue = 0
        do {
            let base = try store.effectiveEngineURL()
            let total = picks.count
            var savedCount = 0
            for (n, entry) in picks.enumerated() {
                statusText = "Pobieranie \(n + 1)/\(total) — start"
                // nazwa na PC oraz docelowa w Documents
                let seriesSuffix = info.entries.count > 1 ? "_\(entry.index)" : ""
                let baseName = AppStore.sanitizeFilename(entry.title.isEmpty ? info.title : entry.title) + seriesSuffix
                let ext = extensionFor(entry: entry)
                let pcPath = "iOS-Downloads/" + baseName + "." + ext
                let formatId = formatIdFor(entry: entry, quality: chosenQuality)

                let start = try await EngineAPI.startDownload(
                    base: base,
                    contentURL: linkText.trimmed(),
                    formatId: formatId,
                    outputPath: pcPath,
                    item: entry.index
                )

                statusText = "Pobieranie \(n + 1)/\(total) — 0%"
                try await pollUntilDone(base: base, jobId: start.jobId, position: n + 1, total: total)
                // GET /file zwraca treść dopiero po completed — plik hotowy
                statusText = "Zapisywanie \(n + 1)/\(total)…"
                let tmp = try await EngineAPI.fetchFile(base: base, jobId: start.jobId)
                _ = try store.saveDownloadedFile(from: tmp, suggestedName: baseName + "." + ext)
                savedCount += 1
            }
            statusText = "Gotowe — \(savedCount)/\(total) zapisano w Pobraniach"
        } catch {
            statusText = "Błąd: \(error.localizedDescription)"
        }
        batchRunning = false
        progressValue = 0
    }

    // Poll /progress co 1 s (limit do 35 min — silnik tnie joba po 30 min).
    private func pollUntilDone(base: String, jobId: String, position: Int, total: Int) async throws -> DownloadProgress {
        var waited = 0
        while waited < 2100 {
            let p = try await EngineAPI.progress(base: base, jobId: jobId)
            let percent = Int(p.progress ?? 0)
            progressValue = Double(percent) / 100.0
            statusText = "Pobieranie \(position)/\(total) — \(percent)%"
            if p.isDone {
                if p.isError {
                    throw EngineError.silnikHTTP(502, p.error.isBlank ? "pobieranie nie powiodło się" : p.error ?? "")
                }
                return p
            }
            try await Task.sleep(nanoseconds: 1_000_000_000)
            waited += 1
        }
        throw EngineError.silnikHTTP(504, "przekroczono limit czasu pobierania")
    }

    // FORMAT per pozycja (kontrakt):
    // - obraz → format_id z pozycji ("img"),
    // - wideo sesyjne → "vid",
    // - wideo yt-dlp → format zgodny z wybraną jakością (ma dźwięk).
    func formatIdFor(entry: Entry, quality: Int?) -> String {
        if entry.isImage { return imageFormatId(entry) }
        if entry.formats.contains(where: { $0.formatId == "vid" }) {
            return "vid"
        }
        let videoFormats = entry.formats.filter { !$0.isImageFormat }
        let withAudio = videoFormats.filter { $0.containsAudio && $0.height != nil }
        if let q = quality {
            if let exact = withAudio.first(where: { $0.height == q }) {
                return exact.formatId
            }
            let lower = withAudio
                .filter { ($0.height ?? 0) <= q && ($0.height ?? 0) > 0 }
                .max { ($0.height ?? 0) < ($1.height ?? 0) }
            if let f = lower {
                return f.formatId
            }
        }
        if let smallest = withAudio.min(by: { ($0.height ?? 0) < ($1.height ?? 0) }) {
            return smallest.formatId
        }
        return videoFormats.first?.formatId ?? "vid"
    }

    private func imageFormatId(_ entry: Entry) -> String {
        if let img = entry.formats.first(where: { $0.isImageFormat }) {
            return img.formatId
        }
        return "img"
    }

    private func extensionFor(entry: Entry) -> String {
        if entry.isImage {
            return entry.formats.first(where: { $0.isImageFormat })?.ext ?? "jpg"
        }
        let chosen = formatIdFor(entry: entry, quality: chosenQuality)
        if let f = entry.formats.first(where: { $0.formatId == chosen }), !f.ext.isEmpty {
            return f.ext
        }
        return "mp4"
    }
}

// MARK: - pomocnicze

extension String {
    func trimmed() -> String {
        trimmingCharacters(in: .whitespacesAndNewlines)
    }
}

extension Optional where Wrapped == String {
    var isBlank: Bool {
        self?.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty ?? true
    }
}
