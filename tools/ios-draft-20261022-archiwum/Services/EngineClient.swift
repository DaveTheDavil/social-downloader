import Foundation

final class EngineClient {
    private let baseUrl = "http://127.0.0.1:8765"
    private let session = URLSession.shared

    func getInfo(url: String) async throws -> VideoInfo {
        guard var components = URLComponents(string: "\(baseUrl)/info") else {
            throw URLError(.badURL)
        }
        components.queryItems = [URLQueryItem(name: "url", value: url)]

        guard let requestURL = components.url else {
            throw URLError(.badURL)
        }

        let (data, response) = try await session.data(from: requestURL)
        guard let http = response as? HTTPURLResponse, http.statusCode == 200 else {
            throw URLError(.badServerResponse)
        }
        return try JSONDecoder().decode(VideoInfo.self, from: data)
    }

    func getAd(type: String) async throws -> AdResponse? {
        guard var components = URLComponents(string: "\(baseUrl)/ads") else {
            throw URLError(.badURL)
        }
        components.queryItems = [URLQueryItem(name: "type", value: type)]

        guard let requestURL = components.url else {
            throw URLError(.badURL)
        }

        let (data, response) = try await session.data(from: requestURL)
        guard let http = response as? HTTPURLResponse, http.statusCode == 200 else {
            return nil
        }
        return try JSONDecoder().decode(AdResponse.self, from: data)
    }

    func getLicense() async throws -> LicenseInfo? {
        guard let requestURL = URL(string: "\(baseUrl)/license") else {
            throw URLError(.badURL)
        }
        let (data, response) = try await session.data(from: requestURL)
        guard let http = response as? HTTPURLResponse, http.statusCode == 200 else {
            return nil
        }
        return try JSONDecoder().decode(LicenseInfo.self, from: data)
    }

    func startDownload(url: String, formatId: String, outputPath: String) async throws -> String? {
        guard let requestURL = URL(string: "\(baseUrl)/download") else {
            throw URLError(.badURL)
        }
        var request = URLRequest(url: requestURL)
        request.httpMethod = "POST"
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        let body: [String: Any] = ["url": url, "format_id": formatId, "output_path": outputPath]
        request.httpBody = try JSONSerialization.data(withJSONObject: body)

        let (data, response) = try await session.data(for: request)
        guard let http = response as? HTTPURLResponse, http.statusCode == 200 else {
            return nil
        }
        if let json = try JSONSerialization.jsonObject(with: data) as? [String: Any] {
            return json["job_id"] as? String
        }
        return nil
    }

    func healthCheck() async -> Bool {
        guard let requestURL = URL(string: "\(baseUrl)/healthz") else { return false }
        do {
            let (_, response) = try await session.data(from: requestURL)
            return (response as? HTTPURLResponse)?.statusCode == 200
        } catch {
            return false
        }
    }

    func getProgress(jobId: String) async throws -> DownloadStatus? {
        guard var components = URLComponents(string: "\(baseUrl)/progress") else {
            throw URLError(.badURL)
        }
        components.queryItems = [URLQueryItem(name: "job_id", value: jobId)]

        guard let requestURL = components.url else {
            throw URLError(.badURL)
        }
        let (data, response) = try await session.data(from: requestURL)
        guard let http = response as? HTTPURLResponse, http.statusCode == 200 else {
            return nil
        }
        return try JSONDecoder().decode(DownloadStatus.self, from: data)
    }
}

struct DownloadStatus: Codable {
    let jobId: String
    let status: String
    let progress: Double
    let error: String?
    let outputPath: String?
    let url: String?

    enum CodingKeys: String, CodingKey {
        case jobId = "job_id"
        case status, progress, error, outputPath = "output_path", url
    }
}
