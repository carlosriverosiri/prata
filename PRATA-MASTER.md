# Prata — Master Document

## Vad Prata är

En minimal Windows-native push-to-talk dikteringsapp för svensk medicinsk diktering med
`KBLab/kb-whisper-large`. Transkribering sker mot en vald backend: en lokal whisper.cpp-GPU-server
över nätverket (**Rngv GPU-server** / **Rum1 GPU-server**) eller **Berget Ai** som moln-fallback.
Designad som komplement till Diktell på maskiner utan dedikerad GPU. Backend-uppsättningen
beskrivs i `PRATA-GPU-SERVER.md`.

## Användarflöde

### F1 — diktering

1. Carlos håller `F1` nere
2. Prata spelar in mikrofon-ljud (16 kHz mono PCM)
3. När `F1` släpps: skicka ljudet till vald backend (Rngv GPU-server / Rum1 GPU-server / Berget Ai)
4. Normalisera svaret till löpande prosa (kollapsa Whispers per-segment-radbrytningar till mellanslag, som Diktell) och tillämpa dictionary-korrigeringar på texten
5. Återställ fönstret som var aktivt när `F1` trycktes ned och skriv texten där via klassbaserad routing (SendInput Unicode i Chromium/Electron-fönster, annars urklipps-paste — se Beslut 6 i designloggen). Om fönstret inte kan återställas avbryts injektionen med felton i stället för att text hamnar fel.
6. Transkribering sker asynkront i en FIFO-worker, så en långsam backend-runda blockerar inte nästa `F1`-inspelning.

### F8 — dictionary quick-fix

1. Carlos markerar ett feltranskriberat ord eller uttryck
2. Trycker `F8`
3. Prata kopierar markeringen och visar en liten popup över markeringen
4. Carlos skriver rätt form och trycker Enter (Esc/klick utanför avbryter)
5. Regeln sparas i per-användarens override-fil (`%LOCALAPPDATA%\Prata\dictionary-corrections.txt`), dictionaryn laddas om, källfönstret återställs och den korrigerade texten klistras tillbaka

## Komponenter

- **Hotkey** — global F1 (PTT) och F8 (dictionary quick-fix) via `RegisterHotKey`
- **Audio capture** — 16 kHz mono PCM via WASAPI (`malgo` Go-binding för miniaudio)
- **HTTP client** — POST multipart till vald backend; OpenAI-kompatibel form (`file`, `model`, `language`, `response_format`)
- **Backend-väljare** — Rngv GPU-server / Rum1 GPU-server / Berget Ai som radioknappar i tray-menyn; aktiv backend syns i tooltip + balong. Valet sparas som stabilt ID (`Hemma` / `Jobb` / `Berget`) i `%LOCALAPPDATA%\Prata\backend.txt` — visningsnamnen kan ändras utan att bryta sparade val. **Standard vid första start (saknad eller ogiltig `backend.txt`): Rum1 GPU-server (`Jobb`)** — intern GPU utan API-nyckel. Villkorlig auth (bara Berget Ai skickar Bearer). Ingen tyst failover. Se `PRATA-GPU-SERVER.md`.
- **Dictionary** — två lager: (1) **baslinje** inbäddad i binären vid build (`go:embed` av `internal/dict/dictionary-corrections.txt`); (2) **per-användare-override** i `%LOCALAPPDATA%\Prata\dictionary-corrections.txt` (F8 skriver hit). Override läggs ovanpå baslinjen (ersätter per nyckel). Unicode-medvetna word-boundary-ersättningar (literal `strings.Index`, ingen regexp).
- **Text injection** — klassbaserad routing: Chromium/Electron (klass `Chrome_WidgetWin_1`, inkl. webbjournalen) → `SendInput` Unicode, hela strängen i ett anrop, urklippet rörs aldrig; övriga fönster → urklipps-paste (`CF_UNICODETEXT`, spara/återställ). Se Beslut 6.

## Berget AI — API-detaljer

- **Endpoint**: `https://api.berget.ai/v1/audio/transcriptions`
- **Modell**: `KBLab/kb-whisper-large`
- **Format**: multipart/form-data
- **Auth**: Bearer token, DPAPI-krypterad lokalt
- **Pris**: €3 per 1000 minuter audio = ~50 öre / månad för Carlos användning
- **Latens** (mätt 2026-05-27 på huvudmaskin):
  - Medel: 2.61 sek
  - Min: 2.56 sek
  - Max: 2.77 sek
  - Spread: 0.21 sek över 5 anrop (mycket konsekvent)
  - Ingen kallstartseffekt observerad

## Mätningar från Fas 0

- Modellen ger **identiskt felmönster** som lokal Diktell (samma KB-Whisper-Large)
- `dictionary-corrections.txt` från Diktell är **direkt återanvändbar** utan modifikation
- Berget AI är ~1.5–2 sek långsammare än lokal RTX GPU för upprepad diktering
- För enstaka dikteringar är skillnaden mindre (lokal har 1850 ms modelladdning vid kallstart)

## Distribution

Två installationsvägar finns parallellt under övergången till en-fil/USB:

| Väg | Mål | Autostart | Status |
|---|---|---|---|
| **`prata.exe --install`** | `%ProgramFiles%\Prata\prata.exe` | Maskinbred Task Scheduler (`Prata`, alla användare, RunLevel Limited) | Implementerad (Fas 5a, ren install) |
| **`install.ps1`** | `%LOCALAPPDATA%\Prata\` | Per-användare Task Scheduler | Legacy / GitHub Releases |

- **Nyckel:** `prata --set-key <key>` (user-scope DPAPI → `%LOCALAPPDATA%\Prata\apikey.dat`). Legacy `prata-setkey.exe` finns kvar tills Fas 7.
- **Skrivbar state** ligger alltid per användare i `%LOCALAPPDATA%\Prata\` (`apikey.dat`, `backend.txt`, dictionary-override). Ingen maskinbred skrivbar data i `%ProgramData%`.
- **Uppdatering:** notifierande (inte automatisk). Tray-menyn har "Sök efter uppdatering…". Själva uppgraderingen sker manuellt (USB-omkörning av `--install` eller legacy `install.ps1`). Binären byter aldrig ut sig själv.
- Versionen stämplas in via `-ldflags "-X main.version=<tag>"` i release-bygget.
- **Hård invariant:** daemonen startas aldrig direkt från den förhöjda installern (HIGH IL → UIPI blockerar SendInput). Post-install-start sker via `schtasks /Run` (medium IL). Se designloggen 2026-06-17.

## Vad Prata ÄR

- Två operationer: `F1` PTT-diktering och `F8` dictionary quick-fix
- Helt lokal förutom HTTP-anropet till vald transkriberings-backend (lokal GPU-server på nätet, eller Berget Ai)
- API-nyckel DPAPI-krypterad på maskinen (behövs bara för Berget Ai-backenden)
- Audio feedback via korta toner: startton (880 Hz) vid inspelningsstart, stopptton (587 Hz) vid släpp, och en felton (dubbel 330 Hz-puls) på de tysta felvägarna i release-kedjan
- Single binary (daemon + `--install` + `--set-key`), ingen runtime, ingen modellfil
- Hårdkodade endpoint-konstanter; backend-*valet* sparas som tillstånd (inte config) i `backend.txt`
- Underhållssubkommandon (`--install`, `--set-key`) rapporterar via `MessageBoxW` (windowsgui = ingen konsol)

## Vad Prata INTE är

- Inte cross-platform (Windows-only just nu — Mac/Linux kan komma senare)
- Inte konfigurerbar (ändra konstant + kompilera om)
- Inte kommersiell — personligt + kollegialt bruk
- Inte ett moln-första system — local-first med moln-fallback för transkribering
- Inte ett ramverk — det är ett verktyg

## Faser

_Ursprunglig plan från Fas 0. Faktiska faser och status — inklusive arbete efter Fas 7
(hybridinjektion, tray-ikon, F8-ordbokstillägg) — dokumenteras i CHANGELOG._

- **Fas 0** — verifiera Berget AI (klar 2026-05-27)
- **Fas 1** — HTTP-klient + WAV-encoding isolerat
- **Fas 2** — audio capture med malgo
- **Fas 3** — hotkey-handling (WH_KEYBOARD_LL)
- **Fas 4** — text injection (SendInput / KEYEVENTF_UNICODE)
- **Fas 5** — dictionary corrections
- **Fas 6** — DPAPI API-nyckel + Task Scheduler autostart
- **Fas 7** — GitHub Actions + install.ps1 (ursprunglig plan)

### Installer-ADR (2026-06-16 — pågående)

| Fas | Innehåll | Status |
|---|---|---|
| 0 | Leveransgren (Gren A: USB, ~12 maskiner, lokal admin) | ✅ Besvarad |
| 1 | Signtool-hook (deferrad) + AV-allowlisting i runbook | ⏳ Hook ej kodad; Defender-undantag i `--install` deferrat |
| 2 | `--set-key` + `MessageBoxW` | ✅ |
| 3 | Ordlista embed + per-användare-override | ✅ |
| 4 | Default-backend Jobb | ✅ |
| 5a | `--install` happy path (ren maskin) | ✅ |
| 5b | Migrering gammal per-användare-install | ⏳ |
| 5c | `--uninstall` | ⏳ |
| 6 | Uppdatering (överskriv medan igång) | ⏳ |
| 7 | Release.yml → en binär + `Installera-Prata.bat` | ⏳ |

## Relation till Diktell

Diktell är "färdig" och fryst. Endast säkerhets- och kraschfixar kommer att tillämpas. All ny
utveckling sker i Prata. Diktell körs på maskiner med GPU; Prata körs överallt annars. De är
systerverktyg, inte versioner.
