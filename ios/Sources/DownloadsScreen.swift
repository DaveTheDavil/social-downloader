import SwiftUI

// Zakładka „Pobrania": pliki zapisane w Documents (tam trafiają wszystkie
// wyniki). Dzięki UIFileSharingEnabled + LSSupportsOpeningDocumentsInPlace
// widać je też w aplikacji Pliki → „Na moim iPhone'ie" → SocialDownloader.
struct DownloadsScreen: View {

    @EnvironmentObject private var store: AppStore

    var body: some View {
        Group {
            if store.savedFiles.isEmpty {
                VStack(spacing: 8) {
                    Image(systemName: "tray.and.arrow.down")
                        .font(.largeTitle)
                        .foregroundColor(.secondary)
                    Text("Brak pobranych plików")
                        .font(.headline)
                    Text("Pliki zapisują się tu automatycznie po zakończeniu pobierania.")
                        .font(.footnote)
                        .foregroundColor(.secondary)
                        .multilineTextAlignment(.center)
                }
                .padding()
                .frame(maxWidth: .infinity, maxHeight: .infinity)
            } else {
                List(store.savedFiles) { file in
                    fileRow(file)
                }
                .listStyle(.plain)
            }
        }
        .navigationTitle("Pobrania")
        .toolbar {
            ToolbarItem(placement: .primaryAction) {
                Button {
                    store.refreshFiles()
                } label: {
                    Image(systemName: "arrow.clockwise")
                }
            }
        }
        .onAppear {
            store.refreshFiles()
        }
    }

    private func fileRow(_ file: SavedFile) -> some View {
        HStack {
            VStack(alignment: .leading, spacing: 2) {
                Text(file.name)
                    .font(.subheadline)
                    .lineLimit(2)
                Text("\(file.sizeText) · \(Self.dateText(file.date))")
                    .font(.caption)
                    .foregroundColor(.secondary)
            }
            Spacer(minLength: 8)
            ShareLink(item: file.url) {
                Image(systemName: "square.and.arrow.up")
            }
        }
        .padding(.vertical, 3)
    }

    private static func dateText(_ date: Date) -> String {
        let fmt = DateFormatter()
        fmt.timeStyle = .short
        fmt.dateStyle = .medium
        fmt.locale = Locale(identifier: "pl_PL")
        return fmt.string(from: date)
    }
}
