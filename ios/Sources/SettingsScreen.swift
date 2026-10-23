import SwiftUI

// Zakładka „Ustawienia": adres silnika (LAN) + test połączenia + krótka
// instrukcja uruchomienia silnika i włączenia portu w zaporze.
struct SettingsScreen: View {

    @EnvironmentObject private var store: AppStore

    @AppStorage(AppStore.engineURLKey)
    private var engineURLSaved = ""

    @State private var urlDraft = ""
    @State private var testState = "" // "ok" | "blad" | ""
    @State private var testDetailedError = ""
    @State private var testing = false

    var body: some View {
        Form {
            Section("Silnik") {
                TextField("Adres silnika", text: $urlDraft, prompt: Text("http://192.168.x.x:8765"))
                    .keyboardType(.URL)
                    .autocorrectionDisabled()
                    .textInputAutocapitalization(.never)
                HStack(spacing: 10) {
                    Button("Zapisz") {
                        saveAndTest()
                    }
                    .buttonStyle(.borderedProminent)
                    Button {
                        Task { @MainActor in await test() }
                    } label: {
                        if testing {
                            ProgressView()
                        } else {
                            Text("Testuj połączenie")
                        }
                    }
                    .buttonStyle(.bordered)
                    .disabled(testing)
                }
                if !testState.isEmpty {
                    HStack(spacing: 6) {
                        Image(systemName: testState == "ok" ? "checkmark.circle.fill" : "xmark.circle.fill")
                            .foregroundColor(testState == "ok" ? .green : .red)
                        Text(testState == "ok" ? "Połączenie OK — silnik odpowiada" : testDetailedError)
                            .font(.footnote)
                    }
                }
            }
            Section("Jak uruchomić silnik na PC") {
                Text("""
                1. Na PC uruchom downloader.exe w katalogu silnika z parametrem:
                   downloader.exe -addr :8765
                2. Sprawdź IP komputera: polecenie ipconfig (szukaj IPv4).
                3. Zgód na dostęp w oknie zapory Windows (port 8765).
                4. Tu wpisz adres: http://IP-KOMPUTERA:8765
                PC i iPhone muszą być w tej samej sieci Wi-Fi oraz IP-KOMPUTERA może się zmieniać — sprawdź po zmianie sieci.
                """)
                .font(.footnote)
                .foregroundColor(.secondary)
            }

            Section("Pliki wynikowe") {
                Text("Pobrania zapisują się w tej aplikacji (zakładka Pobrania) i są widoczne w aplikacji Pliki → „Na moim iPhone'ie" → SocialDownloader. Możesz je tam otworzyć, udostępnić lub przenieść.")
                    .font(.footnote)
                    .foregroundColor(.secondary)
            }

            Section("Sesja Instagram") {
                Text("Sesję IG wklejasz w zakładce Pobieraj (panel na dole) — tam służy do odczytu pełnych serii zdjęć/wideo z Instagrama.")
                    .font(.footnote)
                    .foregroundColor(.secondary)
            }
        }
        .navigationTitle("Ustawienia")
        .onAppear {
            urlDraft = store.engineURL
        }
    }

    private func saveAndTest() {
        UserDefaults.standard.set(urlDraft.trimmingCharacters(in: .whitespacesAndNewlines), forKey: AppStore.engineURLKey)
        Task { @MainActor in await test() }
    }

    private func test() async {
        testing = true
        defer { testing = false }
        do {
            let base = try store.effectiveEngineURL()
            _ = try await EngineAPI.healthz(base: base)
            testState = "ok"
        } catch {
            testState = "blad"
            testDetailedError = error.localizedDescription
        }
    }
}
