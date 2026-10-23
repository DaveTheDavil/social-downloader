import Foundation
import SwiftUI

@main
struct SocialDownloaderApp: App {
    @StateObject private var store = AppStore()

    var body: some Scene {
        WindowGroup {
            RootView()
                .environmentObject(store)
        }
    }
}

// MARK: - układ zakładek

struct RootView: View {
    var body: some View {
        TabView {
            NavigationStack { MainScreen() }
                .tabItem { Label("Pobieraj", systemImage: "arrow.down.circle.fill") }
            NavigationStack { DownloadsScreen() }
                .tabItem { Label("Pobrania", systemImage: "tray.and.arrow.down") }
            NavigationStack { SettingsScreen() }
                .tabItem { Label("Ustawienia", systemImage: "gearshape.fill") }
        }
    }
}

// MARK: - AppStore (wspólny stan między zakładkami)

struct SavedFile: Identifiable {
    let url: URL
    let name: String
    let size: Int
    let date: Date

    var id: String { url.lastPathComponent }

    var sizeText: String {
        ByteCountFormatter.string(fromByteCount: Int64(size), countStyle: .file)
    }
}

// Wspólne stany wszystkich zakładek: lista plików w Documents (widoczne w
// aplikacji Pliki dzięki UIFileSharingEnabled) oraz adres silnika
// (UserDefaults; ustawiane w zakładce Ustawienia).
@MainActor
final class AppStore: ObservableObject {

    static let engineURLKey = "engineURL"

    @Published var savedFiles: [SavedFile] = []

    var engineURL: String {
        (UserDefaults.standard.string(forKey: Self.engineURLKey) ?? "")
            .trimmingCharacters(in: .whitespacesAndNewlines)
    }

    func effectiveEngineURL() throws -> String {
        let raw = engineURL
        guard !raw.isEmpty else { throw EngineError.brakAdresu }
        return raw
    }

    // MARK: Documents / lista plików

    static func documentsDir() -> URL {
        FileManager.default.urls(for: .documentDirectory, in: .userDomainMask)[0]
    }

    func refreshFiles() {
        let docs = Self.documentsDir()
        do {
            let items = try FileManager.default.contentsOfDirectory(
                at: docs,
                includingPropertiesForKeys: [.contentModificationDateKey, .fileSizeKey],
                options: [.skipsHiddenFiles]
            )
            var files: [SavedFile] = []
            for url in items {
                var isDir: ObjCBool = false
                if !FileManager.default.fileExists(atPath: url.path, isDirectory: &isDir) || isDir.boolValue {
                    continue
                }
                let values = try? url.resourceValues(forKeys: [.contentModificationDateKey, .fileSizeKey])
                files.append(SavedFile(
                    url: url,
                    name: url.lastPathComponent,
                    size: values?.fileSize ?? 0,
                    date: values?.contentModificationDate ?? .distantPast
                ))
            }
            files.sort { $0.date > $1.date }
            savedFiles = files
        } catch {
            savedFiles = []
        }
    }

    // Przenosi plik z tymczasowego wyniku URLSession.download do Documents.
    // Nazwa dostaje numer w uniquify, więc elementy batchu nie nadpisują poprzednich.
    func saveDownloadedFile(from tmp: URL, suggestedName: String) throws -> URL {
        var destination = Self.documentsDir().appendingPathComponent(suggestedName)
        destination = Self.uniquify(destination)
        do {
            try FileManager.default.moveItem(at: tmp, to: destination)
        } catch let err as NSError {
            throw EngineError.siec("zapis pliku: \(err.localizedDescription)")
        }
        refreshFiles()
        return destination
    }

    static func uniquify(_ url: URL) -> URL {
        let dir = url.deletingLastPathComponent()
        let name = url.deletingPathExtension().lastPathComponent
        let ext = url.pathExtension
        if !FileManager.default.fileExists(atPath: url.path) {
            return url
        }
        var i = 2
        while true {
            let candidate = dir.appendingPathComponent("\(name) (\(i))" + (ext.isEmpty ? "" : "." + ext))
            if !FileManager.default.fileExists(atPath: candidate.path) {
                return candidate
            }
            i += 1
        }
    }

    // Brakujące znaki na Windows: \ / : * ? | " < > . — cenzura jak w silniku.
    static func sanitizeFilename(_ raw: String) -> String {
        var clean = ""
        for ch in raw {
            let invalid = ch == "/" || ch == "\\" || ch == ":" || ch == "*"
                || ch == "?" || ch == "|" || ch == "\"" || ch == "<" || ch == ">"
                || ch == "." || ch == "\n" || ch == "\r"
            clean.append(invalid ? " " : ch)
        }
        var result = clean
            .components(separatedBy: CharacterSet.whitespaces)
            .filter { !$0.isEmpty }
            .joined(separator: " ")
        if result.count > 60 {
            let cut = result.index(result.startIndex, offsetBy: 60)
            result = String(result[..<cut])
        }
        result = result.trimmingCharacters(in: .whitespacesAndNewlines)
        if result.isEmpty {
            result = "plik"
        }
        return result
    }
}
