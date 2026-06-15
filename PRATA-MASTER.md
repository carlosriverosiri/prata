# Prata — Master Document

## Vad Prata är

En minimal Windows-native push-to-talk dikteringsapp för svensk medicinsk diktering, som
använder Berget AI:s `KBLab/kb-whisper-large` för transkribering. Designad som komplement till
Diktell på maskiner utan dedikerad GPU.

## Användarflöde

1. Carlos håller `F1` nere
2. Prata spelar in mikrofon-ljud (16 kHz mono PCM)
3. När `F1` släpps: skicka ljudet till Berget AI
4. Tillämpa dictionary-korrigeringar på returnerad text
5. Skriv texten i aktivt fönster via klassbaserad routing (SendInput Unicode i Chromium/Electron-fönster, annars urklipps-paste — se Beslut 6 i designloggen)

## Komponenter

- **Hotkey** — global F1 (PTT) och F8 (dictionary quick-fix) via `RegisterHotKey`
- **Audio capture** — 16 kHz mono PCM via WASAPI (`malgo` Go-binding för miniaudio)
- **HTTP client** — POST multipart till Berget
- **Dictionary** — Unicode-medvetna word-boundary-ersättningar (literal `strings.Index`, ingen regexp) från `dictionary-corrections.txt`
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

- GitHub Releases med Windows x86_64 single-binary
- Installation: `install.ps1` som:
  - hämtar senaste release
  - ber om Berget API-nyckel
  - krypterar nyckeln med DPAPI
  - registrerar Task Scheduler-entry för autostart
- **"See and forget"-mål**: ladda ner exe, kör install.ps1, klar

## Vad Prata ÄR

- Två operationer (PTT, möjligen dictionary correction)
- Helt lokal förutom HTTP-anropet till Berget AI
- API-nyckel DPAPI-krypterad på maskinen
- Audio feedback via korta toner: startton (880 Hz) vid inspelningsstart, stopptton (587 Hz) vid släpp, och en felton (dubbel 330 Hz-puls) på de tysta felvägarna i release-kedjan
- Single binary, ingen runtime, ingen modellfil
- Hårdkodade konstanter (ingen `config.toml`)

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
- **Fas 7** — GitHub Actions + install.ps1

## Relation till Diktell

Diktell är "färdig" och fryst. Endast säkerhets- och kraschfixar kommer att tillämpas. All ny
utveckling sker i Prata. Diktell körs på maskiner med GPU; Prata körs överallt annars. De är
systerverktyg, inte versioner.
