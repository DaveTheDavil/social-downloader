# SIGNING-SIDELOAD — iOS: jak podpisywać i ładować apkę bez Apple Developer Program

> **Status projektu iOS: PARKED (2026-10-24, decyzja)** — ten dokument to
> referencja „jak to się robi", gotowa na powrót do tematu. Budowa `.ipa`
> (GitHub Actions, macos runner) opisana jest w **`ios\README-iOS.md`**;
> ten plik dopełnia ją o warstwę podpisu i sideload na Windows.

## Duże zastrzeżenie na start

Free-ruta (zwykłe Apple ID) daje certyfikat ważny **7 dni** i limit
**3 apki na urządzenie na jedno Apple ID** (+ max 10 nowych App ID/tydzień).
Prawdziwe rozwiązanie komercyjne to **Apple Developer Program ($99/rok)** →
Ad Hoc (365 dni) albo TestFlight (90 dni, automatyczne aktualizacje) —
patrz na końcu. Dystrybucja poza App Store = w praktyce tylko sideload.

## Pułapka nr 1: sterowniki Apple na Windows są DWÓCH rodzajów

Narzędzia wymagają **przeciwstawnych** zestawów sterowników:

| Narzędzie | iTunes / iCloud **z MS Store** | iTunes / iCloud **ze strony Apple** (Win32) |
|---|---|---|
| **Sideloadly** | ❌ NIE działa | ✅ **wymagane** |
| **AltStore / AltServer** | ✅ **wymagane** | ❌ NIE działa |

- **Sideloadly:** zainstaluj **iTunes dla Windows (64-bit)** i
  **iCloud dla Windows** pobrane **ze strony apple.com**
  (`iTunesSetup.exe`, `iCloudSetup.exe`) — zawierają `Apple Application
  Support` (64-bit) i `Apple Mobile Device Support`.
- **AltStore:** iTunes + iCloud z **Microsoft Store**.
- **Zmieszanie źródeł = klasyczny błąd** „Apple driver not found". Jeżeli
  wcześniej były wersje z innego źródła → odinstaluj WSZYSTKO (iTunes,
  iCloud, Apple Application Support, Apple Mobile Device Support,
  Bonjour) i postaw od zera jedną ścieżkę. *(To najpewniejsza przyczyna
  wcześniejszych kłopotów z Sideloadly u nas — spróbuj najpierw pełnego
  odinstalowania i postawienia sterowników od zera.)*
- Osobna pułapka: aplikacja **`Apple Devices`** (z MS Store, 2024+) przejmuje
  zarządzanie iPhone'em na USB i kłóci się z iTunes — odinstaluj, dopóki
  nie stoisz w sideload.

## Procedura Sideloadly (Windows, po podłączeniu USB)

1. Telefon odblokowany, kabel **z danymi** (nie tylko ładowanie), pierwszy raz:
   „Zaufaj temu komputerowi".
2. Sideloadly: wskaż `.ipa` (z Actions — patrz README-iOS), pola
   **Apple ID** + hasło.
3. **2FA**: zwykłe hasło wystarcza, Sideloadly przejmuje kod; jeżeli
   klinczuje → hasło aplikacji: appleid.apple.com → Zalogowanie
   i bezpieczeństwo → **Hasła aplikacji** → wygeneruj i wpisz _tam_.
4. **Start** → progress w oknie.

Typowe błędy:

- `Apple driver not found` → pułapka nr 1 (powyżej).
- `Account not verified` → potwierdź logowanie Apple ID (web/telefon).
- `Unable to install` → brak miejsca na telefonie; przekroczony limit
  3 apek na sideload z tego Apple ID (usuń starą apkę); proces potrafi
  trwać kilka minut — nie odłączaj kabla do zakończenia.
- Sideloadly podpisuje SWOIM profilem wygenerowanym z Twojego Apple ID —
  apka pokazuje się na ekranie głównym normalnie.

## Po instalacji: zaufanie do profilu

iOS blokuje apkę z profilu dewelopera do momentu zatwierdzenia:

**Ustawienia → Ogólne → VPN i Zarządzanie urządzeniami → [Apple ID] →
Zaufaj.** Bez tego apka odmawia startu („Niezaufany deweloper").

## Wygasanie 7 dni i renewal

- Po 7 dniach apka przestaje się uruchamiać — **renewal = przepuść .ipa
  jeszcze raz przez Sideloadly** (ten sam Apple ID; profil się odświeża,
  dane apki zostają).
- AltStore/AltServer potrafi renewować **automatycznie**, gdy PC i telefon
  są w jednej sieci + `AltServer` działa w tle (i sterowniki
  iTunes/iCloud z MS Store stoją).

## Apka ↔ silnik (LAN)

- Apka łączy się z silnikiem Go na PC: **`http://<IP-PC>:8765`** (ten sam
  Wi-Fi) — adres wpisujesz w SettingsScreen apki; przycisk **Test**
  odpytuje `/healthz`.
- Windows Firewall: jeśli blokuje, reguła per-port 8765 (profil Prywatny).
- Silnik odpalony z pakiety (`START.bat`) słucha wyłącznie na 127.0.0.1 —
  dla iOS uruchom go z `-addr 0.0.0.0:8765`.
- Telefon w sieci **komórkowej** zamiast Wi-Fi nie znajdzie PC-a (LAN-only).

## Kiedy wyjść z PARKED — opcja „na poważnie"

Apple Developer Program **$99/rok**:

- **TestFlight:** build w GitHub Actions (macos runner), upload do App Store
  Connect; testerzy instalują z linku/maila, **90 dni** ważności builda i
  auto-aktualizacje — bez kabli i bez Sideloadly.
- **Ad Hoc:** podpisana .ipa z CI, rejestracja UDID wybranych iPhoneów,
  ważność 365 dni.
- wymaga: `APPLE_ID`, app-specific password, certy PFX w secrets repo —
  workflow-doc powstanie przy decyzji.

Do tego czasu w `ios\` nie ma kolejnych zmian — ten dokument jest
kompletną instrukcją sideload.
