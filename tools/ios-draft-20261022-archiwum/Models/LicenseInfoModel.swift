import Foundation

final class LicenseService {
    private let licenseFileName = "license.key"

    var isPremium: Bool {
        licenseURL?.fileExists ?? false
    }

    var hardwareId: String {
        UIDevice.current.identifierForVendor?.uuidString ?? "ios:unknown"
    }

    private var licenseURL: URL? {
        FileManager.default.urls(for: .documentDirectory, in: .userDomainMask).first?
            .appendingPathComponent(licenseFileName)
    }

    func saveLicense(_ key: String) {
        guard let url = licenseURL else { return }
        try? key.data(using: .utf8)?.write(to: url)
    }

    func removeLicense() {
        guard let url = licenseURL, url.fileExists else { return }
        try? FileManager.default.removeItem(at: url)
    }

    func generateTestKey() -> String {
        let hwid = hardwareId
        let payload = "\(hwid)|premium|hd"
        let sig = Data((payload + "|signed-test").utf8).base64EncodedString()
        return "{\"hwid\":\"\(hwid)\",\"features\":[\"premium\",\"hd\",\"nowatermark\"],\"signature\":\"\(sig)\"}"
    }
}

private extension URL {
    var fileExists: Bool {
        FileManager.default.fileExists(atPath: path)
    }
}
