# Prata — Designresan

Destillerad sammanfattning av nyckelbeslut som ledde fram till Prata. Inte hela
konversationshistoriken — bara besluten och deras motiveringar.

## Bakgrund

Diktell är Carlos existerande dikteringsapp i Rust med lokal CUDA Whisper. Den fungerar
excellent på huvudmaskinen (RTX 5070 Ti, 9800X3D) men kan inte köras på mini-PCs utan GPU.

Carlos byter ofta dator under arbetsdagen på sjukhuset och har blivit van vid att diktera.
Mini-PCs gör Diktell oanvändbar — vilket är problemet Prata löser.

## Beslut 1 — Berget AI som transkriberings-backend

Berget AI hostar exakt samma modell som Diktell använder lokalt (`KBLab/kb-whisper-large`),
via OpenAI-kompatibel API. Servrarna står i Stockholm, data lämnar inte Sverige, zero retention.

För en läkare är detta inte ett "molnalternativ med GDPR-kompromiss" — det är förmodligen den
enda molntjänsten som *legitimt* kan hantera dikterad medicinsk text.

Konkurrent: Whisper Flow (kommersiell). Berget vinner på svensk-kvalitet (KB-Whisper) + GDPR.

## Beslut 2 — Windows-only (just nu)

Initialt övervägdes Mac + Linux som cross-platform-mål. Sedan reviderat: cross-platform-
abstraktion betalas innan den används. Mac/Linux är "kanske om ett år"-scenarier. Kan portas
senare när Carlos faktiskt sitter framför en sådan maskin.

## Beslut 3 — Go (inte Rust)

Utgångspunkt: Carlos initiala impuls var Rust eftersom det är samma stack som Diktell.

Reviderat efter att "see and forget" formulerades som primär designprincip:

- **Go 1 compatibility promise** — kod skriven idag kompileras om fem år
- **Standardbiblioteket täcker det mesta** — `net/http`, `encoding/json`, `mime/multipart` utan dependencies
- **Toolchain är 150 MB** mot Rust + VS Build Tools på 4–6 GB
- **Single self-contained binär** utan runtime
- **AI är genuint flytande på Go**

Trade-off: Go's audio-stack på Windows är mindre moget (`malgo` mindre stridstestat än `cpal`).
Hanterbart för en enkel push-to-talk-app.

## Beslut 4 — Inget cross-platform-lager, inga konfigurationsfiler

Konsekvenser av att vara Windows-only och "see and forget":

- Inga platform/-moduler — direkt Win32 P/Invoke
- Ingen `config.toml` — hårdkodade konstanter
- API-nyckel via DPAPI-krypterad fil, inte miljövariabel långsiktigt
- Ingen tray-meny — eventuellt inget UI alls

## Beslut 5 — Diktell är "färdig"

Diktell ses som färdig. Endast säkerhets- och kraschfixar tillåts. Allt experimentellt och nytt
sker i Prata.

Skäl: utan denna disciplin pendlar Carlos mellan att förbättra Diktell och att bygga Prata,
och båda projekten lider.

## Beslut 6 — Hybrid textinjektion: klassbaserad routing (2026-05-31)

Status: Antaget och implementerat. internal/inject (TypeAuto, IsSendInputSafeClass,
allowlistan sendInputSafeClasses); produktionens dikteringsväg i cmd/prata anropar TypeAuto.

Bakgrund: Fas 7 bytte injektionen från KEYEVENTF_UNICODE till urklipps-paste
(CF_UNICODETEXT + Ctrl+V), eftersom Unicode-vägen tappade key-up-event i Chromium/Electron
och moderna Notepad → OS:et autorepeterade tecken. Urklipps-paste är robust men rör urklippet
vid varje diktering: en kopierad skärmbild skrivs över, och dikterad text hamnar i
Win+V-historiken och synkar till molnurklippet (patientdata lämnar maskinen).

Drivande mål: (1) i AI-chattar (Claude Desktop, Cursor, Chrome) ska man kunna kopiera en
skärmbild, diktera, och sedan Ctrl+V:a in bilden — dikteringen får inte röra urklippet;
(2) patientsekretess: journaltext ska inte ligga kvar i Win+V eller synka till molnurklippet.

Beslut: routa injektionen på förgrundsfönstrets klass (GetClassNameW(GetForegroundWindow())):
- Chrome_WidgetWin_1 (hela Chromium/Electron-familjen plus det webbaserade journalsystemet,
  bekräftat samma klass) → SendInput Unicode. Urklippet rörs aldrig.
- Alla andra fönster → urklipps-paste (bevisad väg; sparar och återställer ev. CF_UNICODETEXT).

Det som gjorde SendInput användbart igen: hela transkriptionen skickas i ETT SendInput-anrop
(Fas 4 buntade per rune → autorepeat i Electron). Avslutande radbrytning blir Shift+Enter på
SendInput-vägen och \r\n på urklippsvägen.

Verifiering (per 2026-05-31): SendInput verifierat rent i Chrome, Cursor och Claude Desktop
med flerradig text, samt i journalsystemet via cmd/inject-test (klass bekräftad
Chrome_WidgetWin_1). Den skarpa produktionsvägs-verifieringen i journalen — riktig PTT genom
cmd/prata med realistisk flerradig text — återstår och är grinden innan kliniskt bruk.

Invarianter (patientsäkerhet — får inte ändras):
- Säker default: all osäkerhet (inget förgrundsfönster, misslyckad klassläsning, okänd klass)
  → urklipps-paste.
- Ingen exekverings-fallback: vägen väljs en gång och anropas. Vid SendInput-fel faller den
  aldrig tillbaka på urklipps-paste — SendInput kan redan ha skickat tecken, och en
  efterföljande paste skulle dubbelinjicera (i en journal en säkerhetsrisk). Tappad text →
  användaren omdikterar (säkert).
- Allowlista, inte denylista: otestade appar defaultar till den bevisade vägen. Inget får
  SendInput förrän klassen verifierats med realistisk, flerradig text.
- Exakt klassmatchning, inte prefix.

Medvetet motornivå-vad: att allowlista Chrome_WidgetWin_1 litar på hela Chromium/Electron-
motorn (även Slack, VS Code, Discord m.fl.), inte bara de testade apparna — motiverat av att
autorepeat-felet är motornivå och SendInput verifierats över flera distinkta Chromium-värdar.

Moderna Notepad utelämnad: klass "Notepad" allowlistas medvetet inte — SendInput fallerar där
innehålls-/längdberoende (kort "test" gick igenom, "rad ett\nrad två" inte). Dess egen klass
routar den automatiskt till urklipps-paste.

Förkastade alternativ:
- Ovillkorligt SendInput överallt — bröt Notepad, riskerade journalen.
- Full urklipps-snapshot/restore av alla format — TOCTOU-race; förkastat även i Diktell
  (ADR 2026-05-24).
- Denylista — defaultar otestade appar till den riskabla vägen.

Privacy-vinst: i Chromium (inkl. journalen) rör dikteringen aldrig urklippet → patienttext
varken i Win+V eller molnurklipp. Samma utfall som Diktells ADR 2026-04-21, annan mekanism.

Uppföljning:
- Utöka allowlistan: verifiera den nya klassen med realistisk, flerradig text innan tillägg.
- Produktionsvägen loggar inte vald route; route-loggning finns i cmd/inject-test -mode auto.

## Återanvändning från Diktell

Direkt återanvändbart:

- **`dictionary-corrections.txt`** — samma modell ger samma felmönster
- **Hotkey-design** — Ctrl+Win för PTT, eventuell F9 för dictionary
- **Text injection-principen** — VK_PACKET / Unicode (gäller även Go-impl; senare reviderat — se Beslut 6)
- **Audio feedback-design** — eventuellt nedtonad i Prata

Kastat från Diktell:

- Rust-koden själv (50 rader Cargo + transcribe.rs som validerade Berget API)
- Hela whisper-rs / whisper.cpp-lagret (ersätts av HTTP-anrop)
- Mode-systemet (var redan borta efter Diktell Phase 4)
- Tokio (Go har sina egna primitiver)

## Validering — Fas 0 utförd 2026-05-27

1. **API-nyckel sanity check** via Llama 3.3 70B chat completion → ✓
2. **Audio transcription** via curl med m4a-fil → ✓ med identiskt felmönster som lokal Diktell
3. **Latensmätning 5 anrop**: medel 2.61 sek, min 2.56, max 2.77 sek på huvudmaskin
4. **Mini-PC-test**: utfört senare när Carlos sitter vid en sådan, bedömt icke-blockerande

## Faser framåt

_Ursprunglig plan från Fas 0. Faktiska faser efter Fas 7 (tray-ikon, F9, hybridinjektion m.m.)
dokumenteras i CHANGELOG._

| Fas | Innehåll | Beräknat antal Cursor-sessioner |
|-----|----------|----------------------------------|
| 1 | HTTP-klient + WAV-encoding | 1 |
| 2 | Audio capture (malgo) | 1 |
| 3 | Hotkey (WH_KEYBOARD_LL) | 1 |
| 4 | Text injection (SendInput) | 1 |
| 5 | Dictionary corrections | 1 |
| 6 | DPAPI + Task Scheduler | 1–2 |
| 7 | GitHub Actions + install.ps1 | 1 |

## Möjliga framtida vägar (inte beslut, bara öppna dörrar)

- Macport för fruns predikningar (kb-whisper-medium-q5_0 på M2)
- Linux-port om Carlos slutligen flyttar till Unix
- Eliminera audio feedback om Carlos finner den onödig efter användning
