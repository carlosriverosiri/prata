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


### 2026-06-09: PTT moves from Ctrl+Win (WH_KEYBOARD_LL hook) to F1-hold (RegisterHotKey + reconciliation loop)

**Context:**

Prata's `internal/hotkey` was ported conceptually from Diktell: a custom
`WH_KEYBOARD_LL` hook detecting modifier-only Ctrl+Win, with selective Win
suppression and injected-event filtering. The hook approach carries a
documented failure class from six months of Diktell operation: silent
uninstallation when the callback exceeds Windows' ~300 ms
LowLevelHooksTimeout, invalidation across sleep/resume cycles, and AV/EDR
suspicion (keyboard hooks pattern-match keyloggers — relevant on managed
office machines, Prata's primary deployment target). Diktell absorbed this
class with a watchdog thread, generation counters, and recovery machinery;
Prata has none of that safety net.

Separately, distinct PTT gestures let both apps run in parallel on the
development machine: F1 for Prata, Ctrl+Win for Diktell — direct A/B
benchmarking of the two pipelines on the same dictation.

A Diktell observation from April 2026 (ADR 2026-04-21) — bare F-keys via
RegisterHotKey reaching the focused app anyway — argued against this design.
Kanary-tested 2026-06-09 with direct Win32 calls in Go (`cmd/regkey-test`):
WM_HOTKEY is delivered, MOD_NOREPEAT suppresses repeats, 20 ms
GetAsyncKeyState polling detects release with ms precision, and the focused
app (Notepad, browser) never sees F1/F9. Conclusion: the April observation
was a crate-level (`global-hotkey`) artifact, not an API property.

**Alternatives considered:**

- **Keep Ctrl+Win via WH_KEYBOARD_LL (status quo):** identical muscle memory
  to Diktell, zero migration cost. Rejected: inherits the hook failure class
  without Diktell's recovery machinery; porting that machinery contradicts
  Prata's simplification goal; AV/EDR risk on managed office machines;
  blocks parallel A/B with Diktell.
- **Ctrl+Win+Space via RegisterHotKey (MOD_CONTROL | MOD_WIN + VK_SPACE):**
  hook-free, preserves the modifier feel. Rejected for ergonomics (see
  Diktell ADR 2026-04-22) — kept as documented fallback if F1 proves
  Fn-layered on a mini-PC keyboard.
- **F1-hold via RegisterHotKey + reconciliation loop.** Chosen.

**Decision:**

`internal/hotkey` is rewritten around RegisterHotKey: id 1 = VK_F1
(MOD_NOREPEAT) for PTT, id 2 = VK_F9 (MOD_NOREPEAT) for the dictionary
quick-fix. Press = WM_HOTKEY on the registering thread's message queue;
release = 20 ms GetAsyncKeyState polling, started on press, terminated on
release (zero idle cost). The public Listener interface (`NewListener`,
`SetOnF9`, `Run`, `Stop`) is unchanged — `cmd/prata/main.go` is untouched
except user-facing strings. The PTT key is a single const; an env-var
override is deferred until a real Fn-layer problem appears.

**Consequences:**

- The WH_KEYBOARD_LL failure class (silent unhook, sleep/resume
  invalidation, AV signature) leaves the codebase entirely; no watchdog
  will ever be needed.
- The 300 ms callback constraint disappears — callbacks still return fast
  to keep the message loop responsive, but there is no OS-enforced death
  penalty.
- The Ctrl/Win state machine, Win suppression, and LLKHF_INJECTED filtering
  are deleted — RegisterHotKey cannot self-trigger from Prata's own
  SendInput (Ctrl+C/Ctrl+V/VK_PACKET never match F1/F9).
- F1's native function (Help) is consumed system-wide while Prata runs;
  restored on exit. Accepted.
- Prata and Diktell can run simultaneously for A/B benchmarking.
- Different gestures per app/machine — the machine context is itself
  the cue.
- Mini-PC confirmation pending (Fn layer, EDR) with the same
  `regkey-test.exe`; non-blocking, fallback documented above.

  - F9 coexistence with Diktell on the development machine: Diktell's
  WH_KEYBOARD_LL hook consumes F9 before RegisterHotKey matching, so while
  both apps run, F9 deterministically opens Diktell's dictionary popup and
  Prata's quick-fix never fires. Accepted — Prata's F9 is effectively
  office-scoped. Optional refinement: point PRATA_DICT_PATH at Diktell's
  dictionary file on that machine so both apps share one rule set there.

### 2026-06-15: Webroot SecureAnywhere blocks the freshly-built unsigned binary; `go run` is the verified test path

**Context:**

End-to-end dictation was re-verified on a secondary machine: F1-hold →
WASAPI capture → Berget → injection works (four consecutive dictations,
~2.1–2.7 s round-trip, in line with the documented baseline). The earlier
"context deadline exceeded" failures were the Berget outage of
2026-06-10/11, not an app defect — and they are now audible via the
`PlayError` cue added the same week.

Separately, a real deployment obstacle surfaced. A locally built
`prata.exe` placed in the working tree (`C:\Dev\prata`) refuses to launch:
PowerShell reports "not a valid Win32 application" and `cmd.exe` reports
"Access denied" (Swedish: "Åtkomst nekad"). Both are launch-time loader
rejections, not crashes — the Windows Application event log records no
fault from Prata.

The binary itself is sound. The PE was validated by hand: correct `MZ` and
`PE\0\0` signatures, machine type `0x8664` (amd64), full length with
non-zero data through the final bytes (not truncated/gutted). A copy of the
exe under a *different name* is blocked identically, the file carries no
Mark-of-the-Web (no `Zone.Identifier` stream) and no deny ACL, and
`Get-MpThreatDetection` shows nothing.

Root cause: the active security product is **Webroot SecureAnywhere**
(confirmed via the `root\SecurityCenter2` `AntiVirusProduct` class; Windows
Defender is present but passive, and `Get-MpPreference` fails with
`0x800106ba` because WinDefend is not the primary engine). Webroot's
behavioural/journaling model blocks unknown, unsigned, zero-prevalence
executables until it decides they are safe — and a brand-new Go binary that
registers global hotkeys, captures the microphone, and synthesizes
keystrokes is a textbook "suspicious unknown". This is the concrete
materialization of the AV/EDR-suspicion risk anticipated in the
2026-06-09 ADR (one of the motivations for leaving `WH_KEYBOARD_LL`),
except here Webroot blocks the *executable image at launch*, independent of
the hotkey mechanism.

Key asymmetry: `go run ./cmd/prata/` runs fine, because it executes the
compiled binary from the Go build cache under `%LOCALAPPDATA%\go-build`,
which Webroot tolerates, whereas an unsigned exe in a user dev folder is
blocked. (`go run` resolves the dictionary via `os.Executable()` to the
cache directory, so the dictionary soft-degrades to "disabled" unless
`PRATA_DICT_PATH` is set — that env var is the dev workaround, not a bug.)

**Decision (interim):**

For development and testing on Webroot-managed machines, run via
`go run ./cmd/prata/` with `PRATA_DICT_PATH` pointing at the repo's
`dictionary-corrections.txt`. No code change is warranted — the binary is
correct; the obstacle is host policy.

**Options for production deployment (not yet chosen):**

- **Webroot folder/file allowlist (override):** add `%LOCALAPPDATA%\Prata`
  (or the published exe hash) to Webroot's allow list. Simplest for a
  single known machine; does not scale to "see and forget" distribution and
  may require admin/console access on managed hospital machines.
- **Authenticode code signing:** sign `prata.exe` (and `prata-setkey.exe`)
  with a real certificate. The durable fix — a signed, reputable publisher
  identity is what lets Webroot (and SmartScreen, and Defender) trust the
  binary without per-machine overrides. Cost: a code-signing certificate
  (OV/EV) and a signing step in `release.yml`. This is the path that aligns
  with the installer-based "see and forget" goal.
- **Reputation seasoning:** unsigned binaries eventually gain prevalence/age
  and unblock themselves. Unreliable and unacceptable for a clinical tool —
  rejected.

**Consequences:**

- The dictation pipeline is verified working on this machine; the only
  blocker to running the *installed* build is host AV policy, not Prata.
- Code signing is now the leading candidate for the next deployment-
  hardening task and should be folded into the release workflow before
  wider rollout.

### 2026-06-15: Dictionary quick-fix hotkey moved F9 → F8 (Diktell owns F9)

**Context:**

The 2026-06-09 ADR accepted that on a machine running both Diktell and
Prata, Diktell's `WH_KEYBOARD_LL` hook consumes F9 before Prata's
`RegisterHotKey` can match, so Prata's F9 quick-fix never fires there —
"Prata's F9 is effectively office-scoped". In practice the user drives
Diktell with F9 as their primary dictation tool, so the collision is not
theoretical: wherever Diktell runs, Prata's quick-fix is dead on F9.

**Decision:**

Move Prata's dictionary quick-fix from **F9** to **F8**. F8 is unclaimed
by Diktell (which owns F9 and Ctrl+Win) and by Prata's own PTT (F1), so the
two apps get one key each: **F9 = Diktell, F8 = Prata quick-fix, F1 = Prata
PTT**. This supersedes the "office-scoped, accepted" disposition above — the
quick-fix is now expected to work alongside Diktell rather than yield to it.

The change is `vkF8 = 0x77 (VK_F8)` in `internal/hotkey/listener.go`, with
the public API renamed `SetOnF9` → `SetOnF8` and all internal identifiers
(`onF8`, `f8Held`, `f8Busy`, `f8Worker`) and comments following. The test
harness `cmd/f9-test` is now `cmd/f8-test`. `cmd/regkey-test` is left as the
dated F1/F9 RegisterHotKey canary from the 2026-06-09 migration — it records
that diagnostic as run, and is unrelated to the production key choice. The
quick-fix never shipped to users on F9, so no released behavior changes.

**Consequences:**

- On the mini-PC, F8 may sit behind the keyboard's Fn layer (same caveat as
  F1); verified at the next on-device test. `RegisterHotKey` binds the base
  VK_F8, so an Fn-layered keyboard would require Fn+F8.
- The earlier optional refinement (point `PRATA_DICT_PATH` at Diktell's
  dictionary so both apps share one rule set) still stands and is now more
  useful, since Prata's quick-fix can actually run on the shared machine.