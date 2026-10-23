import SwiftUI
import Combine

struct ContentView: View {
    @ObservedObject var viewModel: MainViewModel
    @State private var showingInterstitial = false

    var body: some View {
        ZStack {
            NavigationStack {
                ScrollView {
                    VStack(spacing: 16) {
                        // Disclaimer
                        VStack(alignment: .leading, spacing: 4) {
                            Text("Uwaga: ")
                                .font(.caption2)
                                .fontWeight(.bold)
                                .foregroundColor(.secondary) +
                            Text("Aplikacja służy do pobierania wyłącznie treści, na które masz prawa lub które są w domenie publicznej. Użytkownik ponosi pełną odpowiedzialność za przestrzeganie regulaminów platform i praw autorskich.")
                                .font(.caption2)
                                .foregroundColor(.secondary)
                        }
                        .padding(.horizontal)

                        // Sekcja linku
                        VStack(alignment: .leading, spacing: 8) {
                            Text("Wklej link do filmu lub zdjęcia (Facebook, Instagram, TikTok, YouTube):")
                                .font(.subheadline)
                                .fontWeight(.semibold)

                            HStack(spacing: 8) {
                                TextField("https://...", text: $viewModel.url)
                                    .textFieldStyle(.roundedBorder)
                                    .autocapitalization(.none)
                                    .disableAutocorrection(true)

                                Button("Analizuj") {
                                    Task { await viewModel.analyze() }
                                }
                                .buttonStyle(.borderedProminent)
                                .disabled(viewModel.url.isEmpty || viewModel.isDownloading)
                            }
                        }
                        .padding(.horizontal)

                        // Folder docelowy (iOS: Documents)
                        VStack(alignment: .leading, spacing: 8) {
                            Text("Folder docelowy:")
                                .font(.subheadline)
                                .fontWeight(.semibold)
                            Text(viewModel.outputFolder.isEmpty ? "(brak folderu)" : viewModel.outputFolder)
                                .font(.caption2)
                                .foregroundColor(.secondary)
                                .frame(maxWidth: .infinity, alignment: .leading)
                                .padding(8)
                                .background(Color(.systemGray6))
                                .cornerRadius(6)
                        }
                        .padding(.horizontal)

                        // Wynik analizy
                        if let info = viewModel.videoInfo {
                            VStack(alignment: .leading, spacing: 8) {
                                Text("Wynik analizy")
                                    .font(.headline)

                                Text(info.title)
                                    .font(.headline)
                                    .lineLimit(2)

                                HStack(spacing: 16) {
                                    Label("\(info.duration) s", systemImage: "clock")
                                    Label(info.uploader, systemImage: "person")
                                    Label(viewModel.isPremium ? "Premium" : "Darmowy", systemImage: viewModel.isPremium ? "crown" : "ticket")
                                        .foregroundColor(viewModel.isPremium ? .green : .secondary)
                                }
                                .font(.subheadline)
                                .foregroundColor(.secondary)

                                Text(viewModel.status)
                                    .font(.caption)
                                    .foregroundColor(.secondary)

                                if !viewModel.error.isEmpty {
                                    Text(viewModel.error)
                                        .font(.caption)
                                        .foregroundColor(.red)
                                }
                            }
                            .padding()
                            .background(Color(.systemGray6))
                            .cornerRadius(10)
                            .padding(.horizontal)
                        }

                        // Wybór jakości
                        if let qualities = viewModel.videoInfo?.qualities, !qualities.isEmpty {
                            VStack(alignment: .leading, spacing: 8) {
                                Text("Wybierz jakość:")
                                    .font(.subheadline)
                                    .fontWeight(.semibold)

                                Picker("Jakość", selection: Binding(
                                    get: { viewModel.selectedFormat?.height ?? 720 },
                                    set: { newHeight in
                                        viewModel.selectedFormat = viewModel.videoInfo?.formats.first { $0.height == newHeight }
                                    }
                                )) {
                                    ForEach(qualities, id: \.self) { q in
                                        Text("\(q)p").tag(q)
                                    }
                                }
                                .pickerStyle(.segmented)

                                Text("Dostępne rozdzielczości: \(qualities.count) (p)")
                                    .font(.caption2)
                                    .foregroundColor(.secondary)
                            }
                            .padding(.horizontal)
                        }

                        // Przycisk pobierania i postęp
                        VStack(spacing: 12) {
                            Button("Pobierz plik") {
                                Task { await viewModel.download() }
                            }
                            .buttonStyle(.borderedProminent)
                            .disabled(viewModel.selectedFormat == nil || viewModel.isDownloading)

                            ProgressView(value: Float(viewModel.progress), total: 100)
                                .frame(maxWidth: .infinity)
                        }
                        .padding(.horizontal)

                        // Baner reklamowy (darmowa wersja)
                        if viewModel.showAds, let ad = viewModel.bannerAd {
                            AdBannerView(ad: ad) {
                                viewModel.bannerAd = nil
                            }
                            .padding(.horizontal)
                        }

                        // Licencja premium
                        HStack {
                            Button("Aktywuj wersję premium (test)") {
                                viewModel.activateLicense()
                            }
                            .buttonStyle(.bordered)

                            Spacer()
                        }
                        .padding(.horizontal)

                        Spacer()
                    }
                    .padding(.vertical)
                }
                .navigationTitle("Social Downloader")
            }
            .onChange(of: viewModel.interstitialAd) { _, newValue in
                showingInterstitial = newValue != nil
            }

            // Interstitial overlay (darmowa wersja)
            if showingInterstitial, let ad = viewModel.interstitialAd {
                AdInterstitialView(ad: ad) {
                    showingInterstitial = false
                    viewModel.onInterstitialClosed()
                }
                .transition(.opacity.combined(with: .scale))
                .zIndex(1000)
            }
        }
        .animation(.easeInOut, value: showingInterstitial)
    }
}

struct AdBannerView: View {
    let ad: AdUnit
    let onDismiss: () -> Void

    var body: some View {
        HStack(spacing: 12) {
            if let url = URL(string: ad.image), ad.image.hasPrefix("http") {
                AsyncImage(url: url) { image in
                    image.resizable().aspectRatio(contentMode: .fit)
                } placeholder: {
                    Color.gray.opacity(0.2)
                }
                .frame(width: 60, height: 60)
            } else {
                Rectangle()
                    .fill(Color.gray.opacity(0.2))
                    .frame(width: 60, height: 60)
            }

            VStack(alignment: .leading, spacing: 2) {
                Text(ad.title)
                    .font(.subheadline)
                    .fontWeight(.semibold)
                    .lineLimit(1)
                Text(ad.advertiser)
                    .font(.caption2)
                    .foregroundColor(.secondary)
            }

            Spacer()

            Button {
                onDismiss()
            } label: {
                Image(systemName: "xmark.circle.fill")
                    .foregroundColor(.secondary)
            }
        }
        .padding(10)
        .background(Color.yellow.opacity(0.15))
        .border(Color.yellow, width: 1)
        .cornerRadius(8)
    }
}

struct AdInterstitialView: View {
    let ad: AdUnit
    let onDismiss: () -> Void
    @State private var secondsLeft: Int
    @State private var timer: Timer.TimerPublisher?
    @State private var cancellable: Cancellable?

    init(ad: AdUnit, onDismiss: @escaping () -> Void) {
        self.ad = ad
        self.onDismiss = onDismiss
        self._secondsLeft = State(initialValue: ad.seconds > 0 ? ad.seconds : 5)
    }

    var body: some View {
        ZStack {
            Color.black.opacity(0.6).ignoresSafeArea()

            VStack(spacing: 20) {
                Text("Wiadomość od naszego reklamodawcy")
                    .font(.headline)

                if let url = URL(string: ad.image), ad.image.hasPrefix("http") {
                    AsyncImage(url: url) { image in
                        image.resizable().aspectRatio(contentMode: .fit)
                    } placeholder: {
                        Color.gray.opacity(0.2)
                    }
                    .frame(maxWidth: 200, maxHeight: 150)
                }

                Text(ad.title)
                    .font(.title2)
                    .fontWeight(.bold)
                    .multilineTextAlignment(.center)

                Text(ad.description)
                    .font(.subheadline)
                    .multilineTextAlignment(.center)
                    .foregroundColor(.secondary)

                Text(ad.advertiser)
                    .font(.caption)
                    .foregroundColor(.secondary)

                Button("Przejdź do reklamodawcy") {
                    if let url = URL(string: ad.url) {
                        UIApplication.shared.open(url)
                    }
                    onDismiss()
                }
                .buttonStyle(.borderedProminent)

                Button("Pomiń (\(secondsLeft)s)") {
                    onDismiss()
                }
                .disabled(secondsLeft > 0)
                .onAppear {
                    startTimer()
                }
                .onDisappear {
                    stopTimer()
                }

                Text("Ten ekran znika po wykupieniu wersji premium.")
                    .font(.caption2)
                    .foregroundColor(.secondary)
            }
            .padding(30)
            .background(Color(.systemBackground))
            .cornerRadius(16)
            .shadow(radius: 20)
        }
    }

    private func startTimer() {
        let publisher = Timer.publish(every: 1, on: .main, in: .common)
        cancellable = publisher.autoconnect().sink { _ in
            secondsLeft -= 1
            if secondsLeft <= 0 {
                stopTimer()
            }
        }
    }

    private func stopTimer() {
        cancellable?.cancel()
        cancellable = nil
    }
}
