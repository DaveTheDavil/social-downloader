import Foundation

struct VideoInfo: Codable {
    let id: String
    let title: String
    let duration: Int
    let uploader: String
    let formats: [Format]
    let qualities: [Int]
    let premium: Bool
    let hwid: String
    let features: [String]
}

struct Format: Codable, Identifiable {
    let id = UUID()
    let formatId: String
    let ext: String
    let height: Int
    let width: Int
    let filesize: Int64
    let vCodec: String
    let aCodec: String
    let formatNote: String

    enum CodingKeys: String, CodingKey {
        case formatId = "format_id"
        case ext, height, width, filesize, vCodec = "vcodec", aCodec = "acodec", formatNote = "format_note"
    }
}
