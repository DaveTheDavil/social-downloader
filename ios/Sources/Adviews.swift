import SwiftUI

// Reklamy dla darmowej wersji (silnik: /ads?type=banner|interstitial; gdy
// premium — ad=null albo nie wołamy). UWAGA: AdUnit.image to ścieżka na PC —
// na iOS NIE wczytywać jako obraz; reklamy prezentujemy tekstowo.

// Żółty pasek banerowy: tytuł + sponsor (+ link, jeśli dostępny).
struct BannerAdView: View {
    let ad: AdUnit

    var body: some View {
        Group {
            if let url = ad.targetURL {
                Link(destination: url) {
                    strip
                }
            } else {
                strip
            }
        }
    }

    private var strip: some View {
        HStack(alignment: .top, spacing: 8) {
            Image(systemName: "megaphone.fill")
                .font(.caption)
                .padding(.top, 1)
            VStack(alignment: .leading, spacing: 1) {
                Text(ad.title.isEmpty ? "Reklama" : ad.title)
                    .font(.footnote)
                    .bold()
                    .foregroundColor(.black)
                Text("Sponsor: \(ad.advertiser)")
                    .font(.caption2)
                    .foregroundColor(.black.opacity(0.75))
            }
            Spacer(minLength: 0)
        }
        .padding(9)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(Color.yellow.opacity(0.45))
        .cornerRadius(10)
    }
}

// Interstitial: pełny sheet z odliczaniem ad.seconds (domyślnie 5 s);
// „Pomiń" aktywuje się dopiero po odliczeniu (jak w kliencie Windows).
struct InterstitialView: View {
    let ad: AdUnit
    let onDone: () -> Void

    @State private var remaining: Int
    @State private var countdownRunning = false

    init(ad: AdUnit, onDone: @escaping () -> Void) {
        self.ad = ad
        self.onDone = onDone
        // wartość z silnika; defensywa dla 0/ujemnych
        _remaining = State(initialValue: ad.seconds > 0 ? ad.seconds : 5)
    }

    var body: some View {
        VStack(spacing: 16) {
            Text("Reklama")
                .font(.caption)
                .foregroundColor(.secondary)
                .frame(maxWidth: .infinity, alignment: .leading)
            Text(ad.title.isEmpty ? "Reklama sponsorowana" : ad.title)
                .font(.title2)
                .bold()
                .multilineTextAlignment(.center)
            Text("Sponsor: \(ad.advertiser)")
                .font(.subheadline)
                .foregroundColor(.secondary)
            if !ad.description.isEmpty {
                Text(ad.description)
                    .font(.footnote)
                    .multilineTextAlignment(.center)
            }
            if let url = ad.targetURL {
                Link("Więcej…", destination: url)
                    .font(.subheadline)
            }
            Spacer()
            Button {
                onDone()
            } label: {
                Text(remaining > 0 ? "Pomiń za \(remaining) s" : "Pomiń")
                    .frame(maxWidth: .infinity)
            }
            .buttonStyle(.borderedProminent)
            .disabled(remaining > 0)
        }
        .padding()
        .task {
            guard !countdownRunning else { return }
            countdownRunning = true
            while remaining > 0 {
                try? await Task.sleep(nanoseconds: 1_000_000_000)
                if remaining > 0 {
                    remaining -= 1
                }
            }
            countdownRunning = false
        }
    }
}
