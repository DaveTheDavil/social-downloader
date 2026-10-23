import Foundation

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

    enum CodingKeys: String, CodingKey {
        case id, advertiser, title, description, url, image, type, weight, seconds, clickthrough
    }
}

struct AdResponse: Codable {
    let type: String
    let ad: AdUnit?
    let premium: Bool
}

struct LicenseInfo: Codable {
    let premium: Bool
    let hwid: String
    let features: [String]
}
