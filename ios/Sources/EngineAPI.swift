import Foundation

enum EngineError: Error, LocalizedError {
    case brakAdresu
    case niepoprawnyAdres(String)
    case silnikHTTP(Int, String)
    case dekodowanie(String)
    case siec(String)

    var errorDescription: String? {
        switch self {
        case .brakAdresu:
            return "Ustaw adres silnika w zakładce Ustawienia (np. http://192.168.1.10:8765)."
        case .niepoprawnyAdres(let base):
            return "Niepoprawny adres silnika: \(base)"
        case .silnikHTTP(let code, let body):
            return "[HTTP \(code)] \(body)"
        case .dekodowanie(let why):
            return "Nieoczekiwana odpowiedź silnika: \(why)"
        case .siec(let why):
            return "Błąd połączenia: \(why)"
        }
    }
}

// Klient HTTP silnika (LAN): async/await, Codable, globalna sesja API
// (długie timeouty: /info trwa do 3 minut) + osobna sesja do pobierania plików.
enum EngineAPI {

    private static let apiSession: URLSession = {
        let cfg = URLSessionConfiguration.ephemeral
        cfg.timeoutIntervalForRequest = 210   // /info do 3 min na silniku
        cfg.timeoutIntervalForResource = 3300
        cfg.requestCachePolicy = .reloadIgnoringLocalCacheData
        return URLSession(configuration: cfg)
    }()

    private static let fileSession: URLSession = {
        let cfg = URLSessionConfiguration.ephemeral
        cfg.timeoutIntervalForRequest = 180
        cfg.timeoutIntervalForResource = 7200 // duże wideo/bez limitu logicznego
        cfg.requestCachePolicy = .reloadIgnoringLocalCacheData
        return URLSession(configuration: cfg)
    }()

    private static let decoder: JSONDecoder = {
        let d = JSONDecoder()
        d.keyDecodingStrategy = .convertFromSnakeCase
        return d
    }()

    // MARK: URL helper

    static func makeURL(base: String, path: String, query: [String: String] = [:]) throws -> URL {
        var raw = base.trimmingCharacters(in: .whitespacesAndNewlines)
        if raw.isEmpty { throw EngineError.brakAdresu }
        if !raw.lowercased().hasPrefix("http://") && !raw.lowercased().hasPrefix("https://") {
            raw = "http://" + raw  // wpisanie "192.168.1.10:8765" też działa
        }
        while raw.hasSuffix("/") { raw.removeLast() }
        guard var comps = URLComponents(string: raw) else {
            throw EngineError.niepoprawnyAdres(base)
        }
        comps.path = comps.path + path
        if !query.isEmpty {
            comps.queryItems = query.map { URLQueryItem(name: $0.key, value: $0.value) }
        }
        guard let url = comps.url else {
            throw EngineError.niepoprawnyAdres(base)
        }
        return url
    }

    // MARK: baza odpowiedzi HTTP

    private static func checkStatus(data: Data, response: URLResponse) throws -> Data {
        guard let http = response as? HTTPURLResponse else {
            throw EngineError.siec("brak odpowiedzi HTTP")
        }
        guard (200..<300).contains(http.statusCode) else {
            let text = String(data: data, encoding: .utf8) ?? "(pusta odpowiedź)"
            throw EngineError.silnikHTTP(http.statusCode, text)
        }
        return data
    }

    private static func get(_ url: URL) async throws -> Data {
        do {
            let (data, resp) = try await apiSession.data(for: URLRequest(url: url))
            return try checkStatus(data: data, response: resp)
        } catch let err as EngineError {
            throw err
        } catch {
            throw EngineError.siec(error.localizedDescription)
        }
    }

    // MARK: endpointy

    static func healthz(base: String) async throws -> HealthResponse {
        let url = try makeURL(base: base, path: "/healthz")
        let data = try await get(url)
        do { return try decoder.decode(HealthResponse.self, from: data) }
        catch { throw EngineError.dekodowanie("healthz") }
    }

    static func info(base: String, contentURL: String) async throws -> InfoResponse {
        let url = try makeURL(base: base, path: "/info", query: ["url": contentURL])
        let data = try await get(url)
        do { return try decoder.decode(InfoResponse.self, from: data) }
        catch { throw EngineError.dekodowanie("odpowiedź /info") }
    }

    static func startDownload(
        base: String,
        contentURL: String,
        formatId: String,
        outputPath: String,
        item: Int
    ) async throws -> DownloadStart {
        let url = try makeURL(base: base, path: "/download")
        var req = URLRequest(url: url)
        req.httpMethod = "POST"
        req.setValue("application/json", forHTTPHeaderField: "Content-Type")
        let body = StartRequest(url: contentURL, formatId: formatId, outputPath: outputPath, item: item)
        do {
            req.httpBody = try JSONEncoder().encode(body)
        } catch {
            throw EngineError.dekodowanie("encode /download")
        }
        let (data, resp): (Data, URLResponse)
        do {
            (data, resp) = try await apiSession.data(for: req)
        } catch {
            throw EngineError.siec(error.localizedDescription)
        }
        let ok = try checkStatus(data: data, response: resp)
        do { return try decoder.decode(DownloadStart.self, from: ok) }
        catch { throw EngineError.dekodowanie("odpowiedź /download") }
    }

    private struct StartRequest: Encodable {
        let url: String
        let formatId: String
        let outputPath: String
        let item: Int

        enum CodingKeys: String, CodingKey {
            case url
            case formatId = "format_id"
            case outputPath = "output_path"
            case item
        }
    }

    static func progress(base: String, jobId: String) async throws -> DownloadProgress {
        let url = try makeURL(base: base, path: "/progress", query: ["job_id": jobId])
        let data = try await get(url)
        do { return try decoder.decode(DownloadProgress.self, from: data) }
        catch { throw EngineError.dekodowanie("odpowiedź /progress") }
    }

    // GET /file — plik binarnie (silnik serwuje go dopiero po statusie
    // statusie completed — klient i tak nie wysyła żądań dla niedokończonych
    // zadań). Zwraca tymczasowy URL pliku — klient sam przenosi go do Documents.
    static func fetchFile(base: String, jobId: String) async throws -> URL {
        let url = try makeURL(base: base, path: "/file", query: ["job_id": jobId])
        let (tmpFile, resp): (URL, URLResponse)
        do {
            (tmpFile, resp) = try await fileSession.download(for: URLRequest(url: url))
        } catch {
            throw EngineError.siec(error.localizedDescription)
        }
        guard let http = resp as? HTTPURLResponse else {
            throw EngineError.siec("brak odpowiedzi HTTP")
        }
        guard (200..<300).contains(http.statusCode) else {
            let text = (try? String(contentsOf: tmpFile, encoding: .utf8)) ?? "(pusta odpowiedź)"
            throw EngineError.silnikHTTP(http.statusCode, text)
        }
        return tmpFile
    }

    static func license(base: String) async throws -> LicenseResponse {
        let url = try makeURL(base: base, path: "/license")
        let data = try await get(url)
        do { return try decoder.decode(LicenseResponse.self, from: data) }
        catch { throw EngineError.dekodowanie("odpowiedź /license") }
    }

    static func ads(base: String, type: String) async throws -> AdsResponse {
        let url = try makeURL(base: base, path: "/ads", query: ["type": type])
        let data = try await get(url)
        do { return try decoder.decode(AdsResponse.self, from: data) }
        catch { throw EngineError.dekodowanie("odpowiedź /ads") }
    }

    static func igCookieState(base: String) async throws -> IgCookieResponse {
        let url = try makeURL(base: base, path: "/ig-cookie")
        let data = try await get(url)
        do { return try decoder.decode(IgCookieResponse.self, from: data) }
        catch { throw EngineError.dekodowanie("odpowiedź /ig-cookie") }
    }

    static func saveIgCookie(base: String, cookie: String) async throws -> IgCookieResponse {
        let url = try makeURL(base: base, path: "/ig-cookie")
        var req = URLRequest(url: url)
        req.httpMethod = "POST"
        req.setValue("application/json", forHTTPHeaderField: "Content-Type")
        let payload = ["cookie": cookie]
        req.httpBody = try? JSONSerialization.data(withJSONObject: payload)
        let (data, resp): (Data, URLResponse)
        do {
            (data, resp) = try await apiSession.data(for: req)
        } catch {
            throw EngineError.siec(error.localizedDescription)
        }
        let ok = try checkStatus(data: data, response: resp)
        do { return try decoder.decode(IgCookieResponse.self, from: ok) }
        catch { throw EngineError.dekodowanie("odpowiedź /ig-cookie") }
    }
}
