import Foundation

// Modele JSON silnika (kontrakt: /info, /download, /progress, /file, /license,
// /ads, /ig-cookie). Klucze snake_case z silnika mapowane automatycznie przez
// JSONDecoder.keyDecodingStrategy = .convertFromSnakeCase (patrz EngineAPI).

// MARK: - /healthz

struct HealthResponse: Codable {
    let status: String
}

// MARK: - /info

struct InfoResponse: Codable {
    let id: String
    let title: String
    let duration: Int
    let uploader: String
    let qualities: [Int]
    let formats: [Format]
    let premium: Bool
    let features: [String]
    let hwid: String
    let isPlaylist: Bool
    let entries: [Entry]
    let playlistNote: String
}

// Pozycja publikacji ujednolicona: pojedyncze wideo LUB element serii/serii
// zdjęć (kontrakt v2 silnika).
struct Entry: Codable {
    let index: Int
    let id: String
    let title: String
    let duration: Int
    let isImage: Bool
    let qualities: [Int]
    let formats: [Format]
}

// FORMAT: pola zgodne z engine/internal/downloader/ydlp.go (Format).
// Pola liczbowe opcjonalne — yt-dlp potrafi zwrócić format bez height/width,
// a obrazy syntetyczne są minimalistyczne.
struct Format: Codable {
    let formatId: String
    let ext: String
    let height: Int?
    let width: Int?
    let filesize: Int?
    let fps: Double?
    let vcodec: String?
    let acodec: String?
    let formatNote: String?

    // Łączone formaty silnika ("135+140") zawsze mają dźwięk; wyt-dlp
    // progresywne mp4 zwykle też. Formaty video-only mają acodec == "none".
    var containsAudio: Bool {
        if formatId.contains("+") { return true }
        if let ac = acodec, !ac.isEmpty { return ac != "none" }
        return false
    }

    var isImageFormat: Bool {
        if let vc = vcodec { return vc == "image" || formatId == "img" }
        return formatId == "img"
    }
}

// MARK: - /download + /progress

struct DownloadStart: Codable {
    let jobId: String
    let status: String
}

struct DownloadProgress: Codable {
    let jobId: String
    let status: String
    let progress: Double?
    let error: String?
    let outputPath: String?
    let url: String?

    // Stany zadania w silniku: started | running | completed | failed
    var isDone: Bool { status == JobStatus.completed.rawValue || status == JobStatus.failed.rawValue }
    var isError: Bool { status == JobStatus.failed.rawValue }
}

enum JobStatus: String {
    case started, running, completed, failed
}

// MARK: - /license

struct LicenseResponse: Codable {
    let premium: Bool
    let hwid: String
    let features: [String]
}

// MARK: - /ads
// UWAGA: AdUnit.image jest ŚCIEŻKĄ na PC — na iOS NIE wczytywać jako obraz;
// reklamy wyświetlamy w formie tekstowej (title + advertiser).

struct AdUnit: Codable, Identifiable {
    let id: String
    let advertiser: String
    let title: String
    let description: String
    let url: String
    let image: String
    let type: String
    let weight: Int
    let seconds: Int
    let clickthrough: String?
}

struct AdsResponse: Codable {
    let type: String
    let ad: AdUnit?
    let premium: Bool
}

extension AdUnit {
    // URL docelowy reklamy dla Link — nil, gdy advertiser nie podał linku lub
    // URI jest bez schematu.
    var targetURL: URL? {
        let raw = url.trimmingCharacters(in: .whitespacesAndNewlines)
        if raw.isEmpty { return nil }
        let normalized = raw.lowercased().hasPrefix("http") ? raw : "https://" + raw
        return URL(string: normalized)
    }
}

// MARK: - /ig-cookie
// GET → {"configured":bool}; POST → {"ok":bool,"configured":bool}.
// Wartość cookie NIGDY nie wraca z silnika — pokazujemy tylko status.

struct IgCookieResponse: Codable {
    let ok: Bool?
    let configured: Bool?
}
