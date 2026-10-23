# BUILD_IOS.md — Instrukcja konfiguracji Xcode (klient iOS Social Downloader)

Ten plik opisuje krok po kroku jak utworzyć projekt w Xcode, dodać pliki klienta iOS, skonfigurować sieć (lokalhost / sieć LAN), podpisanie i uruchomić aplikację na fizycznym urządzeniu iOS.

---

## 1. Wymagania wstępne

- macOS z zainstalowanym Xcode (App Store).
- Urządzenie fizyczne z iOS (iPhone / iPad) lub symulator Xcode.
- Apple ID (darmowy wystarczy do trybu deweloperskiego / dystrybucji bezpośredniej).
- Silnik Go działający na komputerze (domyślnie `http://127.0.0.1:8765`) — patrz sekcja **Połączenie z silnikiem Go**.

> **Uwaga:** Kompilacja aplikacji iOS wymaga macOS i Xcode. Nie da się skompilować projektu Swift/SwiftUI na Windows ani Linux.

---

## 2. Tworzenie nowego projektu w Xcode

1. Uruchom Xcode → **Create a new Xcode project**.
2. Wybierz szablon: **App** (pod sekcją iOS) → kliknij **Next**.
3. Konfiguracja projektu:
   - **Product Name:** `Social Downloader`
   - **Team:** wybierz swój Apple ID (lub dodaj go w Xcode → Settings → Accounts).
   - **Organization Identifier:** np. `com.twojanazwa` (dowolny, np. `com.example`).
   - **Interface:** `SwiftUI`
   - **Language:** `Swift`
   - **Storage:** `None` (brak Core Data)
4. Kliknij **Next**, wybierz folder docelowy (np. `C:\Users\DrKu5h\social-downloader\ios\` jeśli pracujesz na macOS z dostępem do tego dysku, lub inny folder na Macu) i kliknij **Create**.

---

## 3. Dodawanie plików z folderu `ios/` do projektu

Pliki źródłowe klienta iOS znajdują się w folderze `ios/` (na Twoim komputerze). Skopiuj je do Maca (np. przez pendrive, sieć udostępnianą lub chmurę), a następnie w Xcode:

1. W projekcie Xcode kliknij prawym przyciskiem na folder projektu w nawigatorze (po lewej) → **Add Files to "Social Downloader"…**.
2. Zaznacz wszystkie pliki z folderu `ios/`:
   - `SocialDownloaderApp.swift`
   - `Models/VideoInfo.swift` (zawiera też `Format`)
   - `Models/AdUnit.swift` (zawiera też `AdResponse` i `LicenseInfo`)
   - `Models/LicenseInfoModel.swift` (`LicenseService`)
   - `Services/EngineClient.swift`
   - `Services/AdService.swift`
   - `ViewModels/MainViewModel.swift`
   - `Views/ContentView.swift` (zawiera `AdBannerView` i `AdInterstitialView`)
3. Zaznacz opcję **Copy items if needed** (skiuj do folderu projektu).
4. Zaznacz cel (target) swojej aplikacji → kliknij **Add**.

> Uwaga: `Format`, `AdResponse` i `LicenseInfo` są zdefiniowane wewnątrz innych plików modeli (nie jako osobne pliki), co jest poprawne w Swift.

---

## 4. Konfiguracja `Info.plist`

### 4.1 App Transport Security (ATS) — dostęp do serwera Go po HTTP

Aplikacja łączy się z silnikiem Go przez zwykłe HTTP (nie HTTPS). iOS domyślnie blokuje połączenia niezaszyfrowane (ATS). Należy dodać wyjątek dla adresu serwera.

Otwórz `Info.plist` i dodaj:

```xml
<key>NSAppTransportSecurity</key>
<dict>
    <key>NSAllowsArbitraryLoads</key>
    <true/>
</dict>
```

lub bardziej bezpiecznie — tylko dla konkretnego hosta (np. adres komputera w sieci):

```xml
<key>NSAppTransportSecurity</key>
<dict>
    <key>NSExceptionDomains</key>
    <dict>
        <key>127.0.0.1</key>
        <dict>
            <key>NSExceptionAllowsInsecureHTTPLoads</key>
            <true/>
        </dict>
        <key>192.168.1.XX</key>  <!-- adres Twojego komputera w sieci LAN -->
        <dict>
            <key>NSExceptionAllowsInsecureHTTPLoads</key>
            <true/>
        </dict>
    </dict>
</dict>
```

### 4.2 (Opcjonalnie) Dostęp do sieci lokalnej

Jeśli urządzenie iOS łączy się przez sieć Wi-Fi do komputera (adres LAN zamiast localhost), dodaj:

```xml
<key>NSLocalNetworkUsageDescription</key>
<string>Aplikacja łączy się z lokalnym serwerem pobierania (silnik Go) przez sieć Wi-Fi.</string>
```

---

## 5. Signing & Capabilities

1. W Xcode wybierz swój target (plik projektu na górze nawigatora) → zakładka **Signing & Capabilities**.
2. Zaznacz **Automatically manage signing**.
3. Wybierz **Team** (Twój Apple ID).
4. **Bundle Identifier:** np. `com.twojanazwa.socialdownloader`.
5. Dla dystrybucji bezpośredniej / Ad Hoc na urządzenie fizyczne:
   - Urządzenie musi mieć włączony **Tryb Deweloperski** (Developer Mode):
     - Na iPhone/iPad: **Ustawienia** → **Prywatność i bezpieczeństwo** → przewiń na dół → **Tryb deweloperski** → włącz → zrestartuj urządzenie.
   - Podłącz urządzenie kablem USB do Maca, zaufaj komputerowi na urządzeniu.
   - W Xcode wybierz swoje urządzenie z listy schemeów (obok przycisku uruchomienia) i kliknij **▶ Run**.
   - Xcode zainstaluje aplikację bezpośrednio (sideload). Darmowe Apple ID pozwala na instalację na własnym urządzeniu (wymaga ponownego zaufania dewelopera co 7 dni w Ustawienia → Ogólne → Zarządzanie urządzeniami).

---

## 6. Uruchomienie na fizycznym urządzeniu iOS

1. Podłącz iPhone/iPad kablem USB do Maca.
2. Na urządzeniu zatwierdź **Zaufaj temu komputerowi**.
3. W Xcode wybierz urządzenie z rozwijanej listy obok przycisku uruchomienia (domyślnie może być wybrany symulator).
4. Kliknij **▶ Run** (lub `Cmd+R`).
5. Aplikacja zostanie skompilowana, podpisana i zainstalowana na urządzeniu.
6. Po pierwszym uruchomieniu, w **Ustawienia → Ogólne → Zarządzanie urządzeniami**, zaufaj deweloperowi (Twój Apple ID), inaczej aplikacja nie otworzy się.

---

## 7. Połączenie z silnikiem Go

### 7.1 Domyślna konfiguracja

W pliku `Services/EngineClient.swift` linia 4:

```swift
private let baseUrl = "http://127.0.0.1:8765"
```

`127.0.0.1` oznacza "to urządzenie" — działa tylko w symulatorze iOS (bo symulator dzieli sieć z Maciem). Na fizycznym iPhone/iPad `127.0.0.1` wskazuje na samo urządzenie iOS, gdzie silnik Go nie działa — połączenie zostanie odrzucone.

### 7.2 Adres komputera w sieci LAN (fizyczne urządzenie iOS)

Aby aplikacja na iPhone/iPad połączyła się z silnikiem Go działającym na Twoim komputerze:

1. Upewnij się, że silnik Go nasłuchuje na wszystkich interfejsach, nie tylko localhost. W `metaai-go/.env` lub przy uruchamianiu ustaw adres `:8765` lub `0.0.0.0:8765` (zależnie od implementacji silnika).
2. Sprawdź adres IP swojego komputera w sieci Wi-Fi, np. `192.168.1.45`.
3. W `EngineClient.swift` zmień:

```swift
private let baseUrl = "http://192.168.1.XX:8765"  <!-- adres Twojego komputera -->
```

4. Zbuduj i uruchom ponownie na urządzeniu.

> Oba urządzenia (iOS i komputer z silnikiem) muszą być w tej samej sieci Wi-Fi.

### 7.3 Endpointy silnika (spójność z modelami JSON)

Klient oczekuje następujących endpointów na porcie `8765`:

| Endpoint | Metoda | Model w Swift | Opis |
|----------|--------|---------------|------|
| `/info?url=` | GET | `VideoInfo` | Metadane filmu: `id, title, duration, uploader, formats, qualities, premium, features, hwid` |
| `/download` | POST (JSON) | `→ {job_id, status}` | Body: `{url, format_id, output_path}` |
| `/progress?job_id=` | GET | `DownloadStatus` | `job_id, status, progress, error, output_path, url` |
| `/license` | GET | `LicenseInfo` | `{premium, hwid, features}` |
| `/ads?type=banner\|interstitial` | GET | `AdResponse` | `{type, ad, premium}` |

Modele JSON w projekcie są zgodne z tym kontraktem (klucze snake_case mapowane przez `CodingKeys`).

---

## 8. Obsługa błędów sieciowych

Jeśli silnik Go nie odpowiada (nie działa, zły adres, brak internetu), aplikacja nie zawiesi się ani nie wyświetli technicznego komunikatu systemowego. Zamiast tego pokazuje przyjazny komunikat po polsku:

- **Brak połączenia z internetem.** — brak sieci na urządzeniu.
- **Nie można połączyć się z silnikiem pobierania. Uruchom serwer Go i sprawdź adres.** — serwer nie działa lub zły adres IP/port.
- **Przekroczono czas oczekiwania na odpowiedź serwera.** — serwer odpowiada z opóźnieniem.
- **Nieprawidłowy adres serwera.** — błędny `baseUrl` w `EngineClient.swift`.
- **Serwer zwrócił nieprawidłową odpowiedź.** — serwer zwrócił inny kod niż 200.
- **Nie można przetworzyć odpowiedzi serwera.** — niepasujący format JSON.

Komunikaty pojawiają się w polu `status` oraz w czerwonym tekście `error` na ekranie analizy i pobierania.

---

## 9. Otwarte problemy i ograniczenia

| Problem | Opis |
|---------|------|
| **Brak możliwości kompilacji bez macOS/Xcode** | Aplikacja iOS (Swift + SwiftUI) wymaga Xcode, który działa tylko na macOS. Na Windows/Linux nie da się skompilować ani uruchomić symulatora iOS. |
| **Hardcoded `baseUrl`** | Adres serwera jest na sztywno w `EngineClient.swift`. Przy zmianie adresu należy ponownie skompilować aplikację. W przyszłości warto dodać ekran ustawień lub polecę plik konfiguracyjny. |
| **Darmowe Apple ID — co 7 dni** | Przy darmowym Apple ID aplikacja zainstalowana bezpośrednio traci zaufanie po 7 dniach i trzeba ją ponownie zaufać w Ustawienia → Zarządzanie urządzeniami (lub ponownie wgrać z Xcode). |
| **`Format.id = UUID()`** | Każdorazowe dekodowanie JSON generuje nowe UUID (tożsamość nie jest stabilna między dekodowaniami). Nie wpływa na funkcjonalność, ale warto wiedzieć przy ewentualnym porównywaniu formatów. |
| **Pliki modeli nie są rozdzielone 1:1** | `Format` znajduje się w `VideoInfo.swift`, a `AdResponse` i `LicenseInfo` w `AdUnit.swift`. To jest poprawne, ale różni się od opisu w zadaniu (które zakłada osobne pliki). |

---

## 10. Szybki test połączenia (smoke test)

Przed uruchomieniem aplikacji upewnij się, że silnik Go odpowiada:

```bash
# Na komputerze z silnikiem (np. w PowerShell):
curl http://127.0.0.1:8765/healthz
```

Oczekiwana odpowiedź: `200 OK` lub puste ciało z kodem 200.

Następnie w aplikacji na iOS wklej dowolny link i kliknij **Analizuj** — jeśli zobaczysz metadane zamiast błędu połączenia, konfiguracja jest poprawna.
