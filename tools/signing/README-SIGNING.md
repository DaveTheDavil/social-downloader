# README-SIGNING — podpisywanie binarek SocialDownloader (Windows)

Stan na **2026-10-24**: pipeline podpisu = **stub** (decyzja: żadnych testowych
self-signed uruchomień i żadnego zakupu teraz). Wszystko jest gotowe do
podpięcia certyfikatu w jednym kroku — bez zmian w skryptach pakowania.

## Co podpisujemy

| Target | Metoda |
|---|---|
| `windows\publish\SocialDownloader.exe` | `signtool sign` |
| `engine\downloader.exe` | `signtool sign` |
| `setup\instalator.ps1` (i jego kopie w stagingu) | `Set-AuthenticodeSignature` |

`.bat` (`INSTALATOR.bat`, `START.bat`) nie podpisuje się — cmd nie ma
mechanizmu podpisu. Certyfikat podpisuje się zawsze z **timestampem RFC3161**
(`signtool /td SHA256 /tr http://timestamp.digicert.com`) — wtedy podpis
pozostaje ważny także po wygaśnięciu certyfikatu, bo czas podpisu potwierdza
zaufany serwer czasowy.

## Jak to działa (pipeline)

`tools\package-windows.ps1` wywołuje **przed stagingiem**
`tools\signing\Sign-Binaries.ps1`:

- **brak konfiguracji** → ostrzeżenie `PODPIS: UNSIGNED (stub)` i pakowanie
  kontynuuje (exit 0) — aktualny tryb,
- **konfiguracja jest** → podpisuje wszystkie targety, timestampuje,
  failuje pakowanie przy błędzie (exit 2 — półpodpisana paczka nie wychodzi),
- `-NoSign` — pomija podpisywanie całkowicie,
- `-Verify` — dodatkowo weryfikuje podpisy (działa też bez konfiguracji,
  jako referencyjny check surowych plików),
- `-Strict` (przyszły CI) — brak konfiguracji = błąd zamiast stuba.

**Plik w użyciu:** jeśli `engine\downloader.exe` (lub appka) działa z repo,
oczekiwane jest `SKIP (plik w uzyciu): engine\downloader.exe - zajmuje:
downloader(PID …)`. Procesy **nie są zabijane** — zamknij je ręcznie i
przelicz pakowanie. Spodziewaj się tego przy każdym dev-teście engine.

**Exit codes `Sign-Binaries.ps1`:** `0` OK/stub · `2` nieudane podpisanie ·
`3` błąd środowiska (brak signtool / PFX nie istnieje / certu brak w store).

## Konfiguracja (tylko zmienne środowiskowe)

Sekrety **nigdy** do repo ani do parametrów CLI:

| Zmienna | Znaczenie |
|---|---|
| `SD_SIGN_PFX` | ścieżka pliku `.pfx` |
| `SD_SIGN_PFX_PASSWORD` | hasło do `.pfx` (**sekret**) |
| `SD_SIGN_SUBJECT` | fragment Subject/CN certu w user-store |
| `SD_SIGN_THUMBPRINT` | thumbprint SHA1 (precyzyjniejsze niż `/n`) |
| `SD_SIGN_TIMESTAMP_URL` | serwer RFC3161 (domyślny: `http://timestamp.digicert.com`) |

Priorytet: `PFX` → `THUMBPRINT` → `SUBJECT`. Ustawienie na stałe (User scope;
wymaga nowego okna konsoli po ustawieniu):

```powershell
[Environment]::SetEnvironmentVariable('SD_SIGN_SUBJECT', 'Imię Nazwisko', 'User')
[Environment]::SetEnvironmentVariable('SD_SIGN_PFX_PASSWORD', '...', 'User')   # sekret: tylko User scope
```

## Trasa A (komercyjna): Certum Standard Code Signing in the Cloud

**Jedyna rozpatrzona trasa dla komercyjnej appki osoby prywatnej w PL** —
polskie CA, klucz w chmurze (SimplySign, bez USB-tokena), wydawca = Twoje
imię i nazwisko. Orientacyjnie **~$115–139/rok** wg resellerów, w
sklep.certum.pl ok. **1 081 zł brutto**; walidacja 3–5 dni: 18+, zdjęcie
dokumentu + skan twarzy + rachunek za media (adres).

Po zakupie:

1. Zainstaluj **SimplySign Desktop** (klucz wirtualny), zaloguj się do
   klucza chmurowego.
2. Cert pojawi się w systemowym store:
   ```powershell
   Get-ChildItem Cert:\CurrentUser\My -CodeSigningCert |
     Select-Object Subject, Thumbprint, NotAfter
   ```
3. Ustaw środowisko: `SD_SIGN_SUBJECT` (albo `SD_SIGN_THUMBPRINT`).
4. Przetestuj: `.\tools\signing\Sign-Binaries.ps1 -Verify`
5. Przepakuj: `.\tools\package-windows.ps1 -Version 1.0.2` — w konsoli
   zamiast `UNSIGNED (stub)` zobaczysz `OK podpisano: …`, a UNSIGNED-warn
   w README pakietu przestaje dotyczyć.

**Ważne (zmiana CA/B od 27.02.2026):** certyfikat code-signing ważny maks.
**459 dni**. Zakupy 2–3-letnie to abonament z **reissue** (bezpłatna
ponowna wystawa tego samego Subjectu). Reissue = instalujesz nowy cert,
`SD_SIGN_SUBJECT` zostaje bez zmian, timestamp robi resztę.

**SmartScreen reputation** buduje się przez **konsekwentne wydania**: ten
sam Subject, podpisane binarki, ciągła seria wydań bez dziur. Od 2024 certyfikaty EV
już nie dają natychmiastowego przepustu — nie dokładaj kosztu.

## Trasa B: Azure Artifact Signing (odroczona)

$9.99/mies. (Basic, 5k podpisów/mies.), klucze w HSM Microsoftu,
integrowanie z CI. **Odroczona**: indywidualne konto tylko USA/Kanada —
w UE ścieżka wymaga **zarejestrowanej organizacji** (JDG/KRS) + Azure
pay-as-you-go. Wraca do gry, gdy powstanie działalność — integracja:
podpis przez usługę Azure zamiast lokalnego signtool; reszta pipeline
(`Sign-Binaries`/pakowanie) zostaje.

## Trasa C (dev): self-signed

`tools\signing\New-SelfSignedTestCert.ps1` — helper **w repo, celowo nie
uruchamiany** (decyzja 2026-10-24). Gdyby przyszła decyzja o teście:
generuje PFX w `%TEMP%\sd-dev-sign\` (poza repo; hasło pokazuje raz na
konsoli), po czym sterowanie tym samymi `SD_SIGN_PFX*`. Wartość = test
mechanizmu; **nie** usuwa „unknown publisher" u użytkowników.

## Drobne fakty

- `signtool`: `C:\Program Files (x86)\Windows Kits\10\bin\<ver>\x64\signtool.exe`
  (skrypt wykrywa sam; `-SigntoolPath` na wymuszanie).
- Podpis weryfikujesz: **Właściwości → Podpisy cyfrowe** albo
  `signtool verify /pa /all <plik>` / `Get-AuthenticodeSignature <plik>`.
- PFX trzymaj poza repo (`%TEMP%\sd-dev-sign\` w trasie C; komercyjne PFX-y
  wg wytycznych CA — zwykle chmura, więc pliku wcale nie ma).
- Hasło PFX: tylko User-scope env. Nigdy w historii konsoli, nigdy
  w skryptach, nigdy w repo.

## iOS

iOS = **PARKED** (doc-only). Ręczny sideload + certy 7-dniowe:
**`ios\SIGNING-SIDELOAD.md`**. Prawdziwy podpis iOS (Apple Developer
Program $99/rok, Ad Hoc/TestFlight z CI macos) — decyzja na przyszłość.
