import SwiftUI

@main
struct SocialDownloaderApp: App {
    @StateObject private var viewModel: MainViewModel

    init() {
        let engine = EngineClient()
        let license = LicenseService()
        let ads = AdService(engine: engine)
        _viewModel = StateObject(wrappedValue: MainViewModel(engine: engine, license: license, ads: ads))
    }

    var body: some Scene {
        WindowGroup {
            ContentView(viewModel: viewModel)
        }
    }
}
