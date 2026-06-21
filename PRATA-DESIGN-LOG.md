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

### 2026-06-15: Update mechanism — notify-only check, not self-update

**Context:**

Prata installs and upgrades through `install.ps1` (GitHub Releases →
`%LOCALAPPDATA%\Prata`, dictionary preserved). Upgrading therefore already
needs no USB stick — re-running the one-liner does it — but nothing tells the
user a new version exists, and the binary carried no version string to
compare against. The question was whether to add an in-app updater, and if so
how much it should do. Cadence is roughly annual; the audience is a handful
of clinical machines; output lands in a patient journal.

**Decision:**

Add a **notify-only** update check, not a self-updater. Three pieces:

1. The binary is stamped with a version via `-ldflags "-X main.version=…"`
   (release workflow uses the git tag; `install.ps1 -Local` uses
   `git describe`; plain `go build`/`go run` stays `"dev"`).
2. `internal/update.Check` queries GitHub's latest-release API and compares
   numeric `vX.Y.Z` versions.
3. A tray item, **Sök efter uppdatering…**, runs the check off the UI thread
   and reports the result in a tray balloon. The actual upgrade is still manual
   (re-running the installer — today `install.ps1`; transitioning to
   `prata.exe --install` on USB, see installer-ADR 2026-06-16).

**Alternatives considered:**

- **Full self-update** (download new exe, rename the running one via
  `MoveFileEx`, write the replacement, restart). Rejected: a binary that
  downloads and executes a replacement of itself is precisely the
  download-and-execute pattern behavioural AV/EDR flags — and the
  unsigned-binary ADR above already documents Webroot blocking Prata at
  launch. Self-update would worsen that surface, add a silent-failure path
  into the one operation that must not go wrong on a clinical tool, and buy
  little for an annual cadence.
- **Silent auto-check on startup.** Reasonable, and easy to add later (the
  `update.Check` + `tray.Notify` plumbing already supports it). Deferred:
  for an annual cadence a constantly-polling background check is overkill,
  and an explicit user action keeps control with the user.
- **Do nothing in the app, document re-running the installer.** Honest
  baseline, but leaves the user with no signal that an update exists.

**Consequences:**

- Once code signing lands (the leading deployment-hardening candidate from
  the ADR above), the notify-only stance can be revisited — a signed binary
  removes the main argument against self-update.
- The check needs network and GitHub's unauthenticated API (60 req/h per IP);
  fine for a manual, occasional click. Failures degrade to a "could not
  check" balloon, never a crash.

### 2026-06-16: En-fil maskinbred installation (Gren A: USB + lokal admin), signering förberedd men deferrad

**Status:** Antaget. **Fas 0 besvarad (2026-06-16): Gren A i småskalig form** —
~10–12 klinikdatorer, inloggade kliniker har lokal admin (UAC fungerar),
distribution via USB-minne manuellt per maskin (inte Intune nu). Designen förblir
förberedd för IT-driven distribution (Intune/SCCM) senare. Fas 2–4 är rena,
osignerade refaktorer som kan köras direkt; Fas 5+ är avblockerade — signering är
inte längre en grind (se beslut 1).

**Bakgrund**

Prata installerades ursprungligen per användare: `install.ps1` kopierar
binärerna till `%LOCALAPPDATA%\Prata` och registrerar en Task Scheduler-uppgift
`"Prata"` för en enskild användare. **Fas 5a (2026-06-17)** lade till
maskinbred install via `prata.exe --install` → `%ProgramFiles%\Prata\` + en
logon-task för alla användare. Båda vägarna finns parallellt tills Fas 7
(städar bort `install.ps1` och legacy-filer). På en klinik med delade PC, där
användare byter dator, är per-användare-modellen fel — varje användare måste
installera om, och separata filer (`prata-setkey.exe`, `dictionary-corrections.txt`,
`install.ps1`) gör paketet ömtåligt. Målet: **en fil** som installerar allt, och
**en installation som gäller samtliga användare** på maskinen.

En arkitektonisk följd av besluten nedan (per-användare-nyckel + per-användare-
ordlista): det finns **ingen maskinbred skrivbar data**. Därför behövs **inget
`%ProgramData%`** — binären ligger skrivskyddad i `%ProgramFiles%\Prata`, all
skrivbar state per-användare i `%LOCALAPPDATA%\Prata`. Det eliminerar hela
ACL-/multisession-write-problematiken.

**Fas 0 — leveransgren (besvarad 2026-06-16: Gren A, småskalig)**

Vem som kör den förhöjda installationen avgör de yttre villkoren, inte
`--install`-logiken (som är **identisk** i båda grenarna):

- **Gren A — klinikern har lokal admin. ← VALD NU.** Self-elevating binär:
  dubbelklick → `ShellExecute "runas"` → UAC → maskinbred install. Skala: ~10–12
  klinikdatorer, distribution via **USB-minne**, manuellt per maskin. Inget
  publikt cert krävs vid denna skala (se nedan + beslut 1).
- **Gren B — ingen clinician-admin (framtida skalning, inte nu).** IT kör samma
  `--install` en gång per maskin, förhöjt, via sitt verktyg (SCCM/Intune/GPO),
  med **IT-allowlisting** (hash/sökväg, eller IT:s eget interna cert i
  EDR/AppLocker) i stället för publik signering. Designen förblir förberedd för
  detta, men det är inte målet nu.

**Varför signering kan deferras nu.** USB-kopierade exe:er saknar normalt
Mark-of-the-Web → SmartScreen triggar inte. Vid denna skala (~12 maskiner) +
lokal admin + USB ersätter **per-maskin-allowlisting** (beslut 9) publik
signering helt. Publikt EV-cert (kräver oftast registrerad organisation) blir
relevant först vid skalning till IT-driven distribution.

**Beslut**

1. **Signering = förberett, deferrat steg (Fas 1) — inte en grind.** Vid den
   valda skalan (Gren A, USB, lokal admin) behövs **inget publikt EV-cert för att
   skeppa**: USB-binärer saknar Mark-of-the-Web (ingen SmartScreen) och
   per-maskin-allowlisting (beslut 9) täcker AV/EDR. Signering implementeras
   därför som en **förberedd hook i `release.yml` som är no-op tills ett cert
   finns**. Detta omvärderar (men river inte) update-ADR:n (2026-06-15):
   self-update förblir avstängt tills en betrodd publisher-identitet finns; den
   körbara distributionen nu är USB + per-maskin-allowlisting. Publikt cert blir
   krav först vid IT-driven skalning (Gren B).
2. **Installationsplats.** Binär i `%ProgramFiles%\Prata` (skrivskyddad för
   icke-admin — daemonen kan inte modifiera sin egen image). All skrivbar state
   per-användare i `%LOCALAPPDATA%\Prata`. **Inget `%ProgramData%`.**
3. **Berget-nyckel.** Behåll per-användare user-scope DPAPI (status quo) via
   `prata --set-key`. **Ingen** `CRYPTPROTECT_LOCAL_MACHINE` — det skulle
   exponera nyckeln för alla på en delad PC. Krävs ej för Jobb/Hemma.
4. **Ordlista.** `go:embed` av delad baslinje + per-användare-override i
   `%LOCALAPPDATA%\Prata`. Sidesteppar både skrivrättighet i ProgramFiles och
   multisession-write-racen mot en delad fil. F8 skriver till overriden.
   `resolvePath` (dict.go) **och** `loadDict` (main.go) räknar idag ut sökväg
   oberoende och **måste ändras tillsammans**. En **byggtidsrutin** designas för
   att vika in värdefulla override-tillägg i baslinjen vid release
   (klinikkorrigeringar är domänkunskap, inte personlig preferens);
   implementationen får faslägga, men gränssnittet designas.
5. **Default-backend Jobb.** `loadBackendPref`-defaulten ändrades Berget → Jobb
   (implementerad Fas 4); `backend.txt` per-användare överrider. Annars träffar en
   ny användare Berget-utan-nyckel vid F1 → felton.
6. **Autostart.** En maskinbred Task Scheduler-uppgift, trigger AtLogon för
   **alla** användare (Principal `BUILTIN\Users` med implicit interaktiv logon,
   RunLevel Limited), startar Prata i varje användares session. **Task Scheduler
   > HKLM\Run** motiveras av RunLevel-kontroll (medium IL, se invariant),
   startvillkor och robusthet; HKLM\Run nämns som enklare fallback.
7. **Migration (spänner alla profiler; data bara för installerande användare).**
   `--install` upptäcker och städar bort tidigare per-användare-install. Cleanup
   av gamla autostarter **måste spänna samtliga användarprofiler** — admin kan
   enumerera och ta bort gamla `"Prata"`-tasks och `%LOCALAPPDATA%`-exe-kopior
   tvärs alla användare. Men **per-användare-DATA kan inte migreras tvärs
   användare:** `apikey.dat` är user-scope DPAPI och är oläsbar för
   installeraren. Endast den **installerande användarens** data migreras (Gren
   A: bevara `apikey.dat`/`backend.txt`, migrera ev. gammal ordlista →
   override). Övriga användare får **färska defaults vid första körning** —
   acceptabelt, eftersom Jobb inte kräver nyckel och ordlistebaslinjen är
   embeddad. `--uninstall` tar bort ProgramFiles-mappen + den maskinbreda
   tasken. (Rör **inte** `PrataWhisperServer` — det är GPU-serverns task, en
   annan sak.)
8. **En (1) binär.** Leveransen är **en enda binär** med Jobb-default inbyggd +
   per-användare `backend.txt`-override — **inte** separata namngivna builds per
   plats eller per gren. Samma `prata.exe` kör daemon, `--install`,
   `--uninstall` och `--set-key`.
9. **AV/EDR-allowlisting (del av install-rutinen).** Designloggen dokumenterar
   att Webroot blockerar osignerade binärer vid start (ADR 2026-06-15). Två
   vägar designas, så installationen funkar oavsett vilket skydd maskinen kör
   (vilken AV bekräftas med IT):
   - **Windows Defender:** den förhöjda `--install` lägger undantaget själv —
     `Add-MpPreference -ExclusionPath "%ProgramFiles%\Prata"` — under den
     befintliga UAC-förhöjningen, ingen extra prompt.
   - **Tredjeparts-EDR (Webroot e.d.):** undantaget kan inte sättas
     programmatiskt; det görs i EDR-konsolen och dokumenteras som ett steg i
     **USB-runbooken**.

**Invarianter (patientsäkerhet — får inte ändras)**

- **UIPI / medium IL.** Daemonen kör på medium IL (Task Scheduler RunLevel
  Limited). **Bara** install-åtgärden förhöjer. En förhöjd daemon bryter
  SendInput-injicering i ett icke-förhöjt Webdoc **tyst** — hård invariant.
- **windowsgui = ingen konsol.** All installer-/update-feedback via `MessageBoxW`
  (inkl. "UAC avbruten", fel, klart). `--set-key` som **ren argform**
  (`--set-key <key>`), ingen interaktiv prompt.
- **Single-instance-mutexen är redan sessionsbunden** (oprefixat namn i
  `single.Acquire` = `Local\`). Verifieras och dokumenteras — ändras inte. Det
  är detta som gör att Prata får en instans *per session* på en delad PC.

**Alternativ som förkastades**

- **HKLM\Run** i stället för Task Scheduler — ingen RunLevel-/villkorskontroll,
  kan stängas av per användare i Aktivitetshanteraren. Behålls bara som
  nödfallback.
- **Machine-scope DPAPI** för Berget-nyckeln — exponerar hemligheten för alla på
  maskinen; onödigt då Berget är nedprioriterad.
- **Delad skrivbar ordlista i `%ProgramData%`** — kräver ACL-vidgning + atomisk
  write + tvärprocess-lås mot multisession-race; mycket maskineri mot
  minimalism. Per-användare-override ger samma nytta utan racen.
- **MSI/Inno/NSIS/WiX** — externa paketeringsverktyg bryter
  en-fil-/stdlib-only-principen.
- **Separata namngivna builds per plats/gren** — bryter en-binär-principen;
  ersätts av Jobb-default + per-användare-override (beslut 5 och 8).

**Konsekvenser**

- **Signering är inte kritisk väg nu.** Med Gren A/USB/allowlisting byggs Fas
  2–4 osignerat och Fas 5+ är avblockerade. Cert blir kritisk väg först vid
  IT-driven skalning (Gren B). EV-cert-ledtiden stallar inget kodbart arbete.
- `%ProgramFiles%`-placeringen gör att en körande exe inte kan skriva över sig
  själv → uppdatering (manuell USB-omkörning) måste stoppa task + alla instanser,
  kopiera, omregistrera, starta om (Fas 6). Ingen nedladdning — inte network
  self-update.
- **Post-install-start är interaktivt-only.** "Starta i aktuell session efter
  install" gäller den valda **Gren A** (interaktiv UAC-förhöjning) — fungerar nu.
  Skulle `--install` framöver köras som SYSTEM via SCCM (**Gren B**) finns ingen
  interaktiv session, och Prata startar då först vid nästa inloggning via tasken.
  Startsteget greenas så det inte felar under SYSTEM-kontext (förväntat,
  icke-fatalt).
- **Multisession:** den maskinbreda tasken startar Prata i varje session vid
  inloggning. Redan inloggade sessioner uppdateras/startar först vid nästa
  inloggning.
- **Blast radius:** `release.yml` (skeppar idag `prata.exe`, `prata-setkey.exe`,
  `dictionary-corrections.txt`, `install.ps1`), update-ADR:n och tray-strängen
  ("Kör om installationskommandot") måste uppdateras i takt (Fas 1/6/7).
- "See and forget" och minimalism bevaras genom att install-/update-kodvägen
  hålls **strikt isär** från daemon-hot-pathen — runtime förblir minimal även
  när binären får ett install-läge.

**Faslagd plan (sammanfattning)**

- **Fas 0** — BESVARAD (2026-06-16): Gren A, ~12 maskiner, USB, lokal admin
  finns. Avblockerad.
- **Fas 1** — Signtool-hook i `release.yml` (**deferrad, no-op tills cert finns**)
  + USB-install-rutin/runbook med AV-allowlisting (Defender via
  `Add-MpPreference` i `--install`; tredjeparts-EDR i konsolen). **Inte längre en
  grind** för Fas 5+.
- **Fas 2** — `--set-key` som subkommando (ren argform) + `MessageBoxW`-helper.
  ✅ Implementerad.
- **Fas 3** — Ordlista: `go:embed` baslinje + per-användare-override. ✅
  Implementerad.
- **Fas 4** — Default-backend Berget → Jobb. ✅ Implementerad.
- **Fas 5a** — `--install` happy path (ren maskin, self-elevating). ✅
  Implementerad (2026-06-17).
- **Fas 5b** — Migrering gammal per-användare-install. I det förhöjda
  `installElevated`, **före** copyFile: (1) `terminateOtherInstances` dödar varje
  körande `prata.exe` utom den egna PID:en (`CreateToolhelp32Snapshot` →
  `Process32FirstW/NextW`, `OpenProcess(PROCESS_TERMINATE)` + `TerminateProcess`)
  — röjer både den sessionsbundna single-instance-mutexen och ev. fillås på
  målbinären; (2) `copyFileWithRetry` (10 × 200 ms) tål ett övergående lås efter
  termineringen och avbryter installen med felruta om låset kvarstår (aldrig tyst
  fortsättning); (3) efter `schtasks /Run` städar `cleanupLegacyUserBinaries` bort
  **endast** `prata.exe` + `prata-setkey.exe` i varje `C:\Users\*\AppData\Local\
  Prata\` (best-effort; användardata bevaras). Task-XML, RunLevel och medium-IL-
  starten är **orörda** → invarianten intakt. Self-PID-exkludering är obligatorisk.
  Status: **✅ hårdvaruverifierad 2026-06-20** — dirty-state-smoke-test:
  `terminateOtherInstances` dödade den gamla daemonen (PID 82948),
  `copyFileWithRetry` absorberade fillåset (försök 1 → 2), ny daemon (PID 99272)
  från `%ProgramFiles%` blev kvar, F1-injicering i ett oförhöjt fönster fungerade
  (medium IL) och användardata bevarades. Känd begränsning:
  binären man kör `--install` *från* kan inte raderas av städningen (i bruk) →
  loggas, ofarlig eftersom maskinvida binären i `%ProgramFiles%` är auktoritativ.
- **Fas 5c** — `--uninstall` (speglar `--install`). `Uninstall()` självförhöjer
  via `relaunchElevated("--uninstall")` (helpern parametriserades; `Run` skickar
  `"--install"` — beteendebevarande). Förhöjt, i ordning: (1)
  `terminateOtherInstances` stoppar daemonen + låser upp binären (self-PID
  exkluderad); (2) `schtasks /Delete /TN "Prata" /F`, klassad **locale-säkert**
  via post-state `schtasks /Query` (`taskAbsent`) — aldrig felsträngsparsning; (3)
  `removeInstallDirWithRetry` tar bort `%ProgramFiles%\Prata` (10 × 200 ms mot det
  övergående image-section-låset efter terminering). **Per-användardata i
  `%LOCALAPPDATA%\Prata` lämnas orörd** (API-nyckel, ordlista, backend-val — dyra
  att återskapa, och `--install` skapade dem aldrig; symmetri). `PrataWhisperServer`
  (whisper-serverns SYSTEM-task) rörs aldrig — endast `"Prata"` adresseras. Best-
  effort teardown: "redan borta" = lyckat, genuin kvarleva → mjuk varnings-
  MessageBox (ingen krasch). **Körande-binär-begränsning (Option A):** körs
  `--uninstall` från den *installerade* `%ProgramFiles%\Prata\prata.exe` kan mappen
  inte tömmas helt (Windows låter inte radera en körande `.exe`); `runningFromInstallDir`
  upptäcker detta och visar "kör --uninstall från USB-/originalkopian". Robustare
  temp-kopia-återstart (Option B) byggs inte nu. Status: **✅ hårdvaruverifierad
  2026-06-20** — smoke-test (`--uninstall` från extern `C:\Dev`-kopia, inte den
  installerade binären): `terminateOtherInstances` dödade den körande daemonen
  (PID 99272), `schtasks /Query` bekräftade tasken borta, `%ProgramFiles%\Prata`
  togs bort, och `%LOCALAPPDATA%\Prata` lämnades intakt (6 filer kvar: apikey.dat,
  backend.txt, ordlistan + `.default` + `.bak`, prata.exe.bak). Eftersom uninstall
  kördes utanför installDir gällde lyckade-vägen, inte Option-A-varningen.
- **Fas 6** — Uppdateringsflöde. **Mekaniken finns redan i `--install`** och kräver
  ingen ny kod: `installElevated` kör `terminateOtherInstances` → `copyFileWithRetry`
  → `registerTask` (`schtasks /Create /F`, så XML:en regenereras och en ändrad
  task-definition appliceras) → `runTask` (omstart i sessionen, medium IL). Bevisat av
  5b-smoke-testet (överskriv-medan-igång: dödade PID 82948 → retry-copy → omregistrering
  → omstart). En uppdatering = **kör om `--install` från den NYA binären på USB-minnet**;
  `samePath(src,dst)`-vakten gör att om man kör den redan installerade
  `%ProgramFiles%\Prata\prata.exe --install` hoppas kopian över (bara task-reparation +
  omstart, ingen versionshöjning) — uppdatering måste därför ske från USB-kopian, inte
  från den installerade binären (samma modell som uninstall Option-A). Fas 6 är därför
  **bara text + docs**: tray-notisen (`res.Newer`) pekar nu på USB-omkörningen i stället
  för det vaga "installationskommandot", och de stale `install.ps1`-kommentarerna i
  `internal/update` + `main.go` korrigeras (installern är en separat process som dödar
  daemonen *före* kopian → ingen self-overwrite, ingen rename-dans). Versions-checken
  (`update.Check`) är oförändrad. **Multi-session-villkor:** `terminateOtherInstances`
  dödar *alla* andra `prata.exe` per namn (self-exkluderad), så på en delad klinikdator
  avbryter en `--install` även en annan inloggad användares daemon — uppdatera när ingen
  dikterar (full USB-runbook-rad = Fas 7). Status: **✅ Verifierad 2026-06-20** (grindar + diff-granskning; ren
  strängändring, ingen ny mekanik att hårdvarutesta — uppdateringsmekaniken är
  redan 5b-verifierad).
- **Fas 7** — Paketering + legacy-städning. `release.yml` skeppar nu **EN binär**
  (`prata.exe`) + USB-wrappers `Installera-Prata.bat`/`Avinstallera-Prata.bat`;
  `prata-setkey.exe`, rot-`dictionary-corrections.txt` och `install.ps1` släpps ur
  release-bunten. Borttaget ur repot: `install.ps1`, `cmd/prata-setkey/` (infälld i
  `prata --set-key` sedan Fas 2) och rot-dubbletten av ordlistan (embed-källan
  `internal/dict/dictionary-corrections.txt` är enda sanningen). Signering är en
  **förberedd, deferrad hook** i `release.yml` (gated på `CODE_SIGN_PFX`-secret,
  no-op utan cert). `logf`-sökvägen är nu env-styrbar via `PRATA_INSTALL_LOG`
  (testisolering + dev). Docs synkade (README, PRATA-MASTER, GPU-SERVER, CHANGELOG).
  Legacy-städningskommentaren i `installer.go` (`cleanupLegacyUserBinaries`) lämnas —
  den städar redan utplacerade `prata-setkey.exe` på klinikdiskar, inte nuvarande metod.
  Status: **✅ 2026-06-20** — kod + docs klara; `.bat` hårdvaru-röksstestad (launch +
  å/ö + pause-nät); `release.yml` review-verifierad (full validering på första `v*`-taggen).

### 2026-06-16: Ordlista — embeddad baslinje + per-användare-override (Fas 3 implementerad)

**Status:** Implementerad. Genomför Fas 3 i installer-planen ovan.

**Vad som gjordes**

- Baslinjen (`dictionary-corrections.txt`) `go:embed`:as nu i binären som en
  **immutabel** lager (`internal/dict/dictionary-corrections.txt`, byte-identisk
  kopia av rot-filen). Den laddas alltid — ordlistan kan inte längre "tyst
  inaktiveras" för att en fil saknas bredvid exet.
- **Override** läggs ovanpå baslinjen (`dict.LoadDefault` → `loadLayered` →
  `mergeRules`): en override-post **lägger till** eller **ersätter per nyckel**
  en baslinjepost. Ersättning sker på första (och enda eldande) förekomsten, så
  override vinner under first-match-wins.
- **Sökvägsupplösning enad.** `resolvePath` (dict) returnerar OVERRIDE-sökvägen:
  `PRATA_DICT_PATH` (dev) → annars `%LOCALAPPDATA%\Prata\dictionary-corrections.txt`.
  `cmd/prata`s `loadDict` delegerar till `dict.LoadDefault` — de räknar inte
  längre ut sökväg oberoende, så daemon/`Save`/`Reload` är alltid överens.
- **F9/`dict.Save` skriver ENDAST till override-filen** (skapar
  `%LOCALAPPDATA%\Prata` vid behov), aldrig baslinjen, aldrig bredvid exet.
- **Biverkan löst:** `go run`-quirken (`os.Executable` → byggcache → ordlistan
  inaktiverades) är borta eftersom baslinjen alltid är embeddad.
- **Transient dubblett:** rot-`dictionary-corrections.txt` **borttagen 2026-06-20
  (Fas 7)** — den var byte-identisk med embed-källan och skeppades tidigare av
  `release.yml`/`install.ps1`; ofarlig att ta bort eftersom runtime aldrig läste
  bredvid exet. `internal/dict/dictionary-corrections.txt` är enda baslinjekällan.

**Byggtids-fold-in — GRÄNSSNITT designat nu, implementation faslagd (Fas 5/6)**

Värdefulla override-poster ska kunna "vikas in" i den embeddade baslinjen inför
en release, så att de skeppas till alla användare. Kontraktet:

- **Form:** ett litet Go-CLI, `cmd/dict-foldin` (stdlib-only, ingen
  daemon-koppling), körs **manuellt av utvecklaren** före en release-build —
  inte i daemon-hot-pathen, inte automatiskt i CI.
- **Anrop:**
  `dict-foldin --override <path> [--baseline internal/dict/dictionary-corrections.txt] [--dry-run]`
  - `--override` (obligatorisk): källan att vika in (typiskt en användares
    `%LOCALAPPDATA%\Prata\dictionary-corrections.txt`).
  - `--baseline` (default `internal/dict/dictionary-corrections.txt`): målet som
    embeddas vid nästa build — den **enda** baslinjekällan.
  - `--dry-run`: skriv ut diffen (skulle-läggas-till / skulle-ersättas), skriv
    inget.
- **Semantik:** identisk med runtime-`mergeRules` — per nyckel lägg till eller
  ersätt på plats; bevara kommentarer, tomrader och ordning i baslinjen; hoppa
  över tomma/identitetsregler (som `Save`). **Tar aldrig bort** baslinjeregler.
- **Utdata:** uppdaterad baslinjefil (idempotent) + en kort rapport
  (added/replaced/skipped). Exit ≠ 0 vid parsefel i någon fil.
- **Invariant:** baslinjen förblir den enda embeddade källan; verktyget
  redigerar bara den filen, rör aldrig användarens override.

### 2026-06-17: `--install` maskinbred, self-elevating — happy path (Fas 5a implementerad)

**Status:** Implementerad (ren install, ingen tidigare Prata). Genomför Fas 5a.
Deferrat: migrering av per-användare-install (5b), `--uninstall` (5c),
överskriv-medan-igång/uppdatering (6), Webroot-allowlisting + `Installera-Prata.bat`
(7).

**Vad som gjordes**

- Nytt paket `internal/installer` (rå `syscall`, ingen ny dependency — håller
  stdlib-only-principen). `dispatchSubcommand` fick `case "--install"`.
  No-args = daemon är oförändrat.
- **Förhöjning:** `isElevated` (`OpenProcessToken` + `GetTokenInformation`
  `TokenElevation`). Ej förhöjd → `ShellExecuteW` verb `runas` params
  `--install`, exit; retur ≤ 32 (UAC nekad) → svensk MessageBox. Redan förhöjd
  (återstartat barn / Gren B) → fortsätt. `isElevated`-kollen hindrar loop.
- **Kopiering:** `os.Executable()` → `%ProgramFiles%\Prata\prata.exe`.
  source==dest jämförs på normaliserad, case-insensitiv sökväg → hoppa kopian
  men omregistrera tasken (idempotent reparation). Låst/oskrivbart mål → fel,
  ingen tyst fortsättning.
- **Maskinbred task** via genererad XML (UTF-16LE + BOM, `schtasks /Create /XML
  … /F`): `LogonTrigger` utan `UserId`, `GroupId` = `S-1-5-32-545`,
  `RunLevel` `LeastPrivilege`, `MultipleInstancesPolicy` `Parallel`,
  `ExecutionTimeLimit` `PT0S`. **Ingen explicit `LogonType`:** för en
  grupp-principal är interaktiv logon implicit, och v1.2-schemat kräver
  `LogonType` *före* `GroupId` — ett explicit värde gav `schtasks`-felet
  "unexpected node" (rättat 2026-06-20; elementordningen vaktas nu av ett
  enhetstest). Medium-IL-garantin ligger i `RunLevel`, inte i `LogonType`.
- **Post-install-start:** `schtasks /Run /TN "Prata"` (best-effort, medium IL).
  Misslyckas → icke-fatalt ("nästa inloggning").

**Beslut värda att notera**

- **GroupId via SID `S-1-5-32-545`, inte literalen "Users"/"BUILTIN\\Users".**
  Gruppens *visningsnamn* är lokaliserat (svensk Windows: "Användare"); den
  välkända SID:en är språkoberoende och alltid upplösbar. Korrekt teknik trots
  att prompten skrev "BUILTIN\\Users".
- **`MultipleInstancesPolicy` = `Parallel`** (inte `IgnoreNew`): en instans per
  session i multisession; sessionsmutexen hindrar dubletter inom en session.
  `IgnoreNew` hade kunnat blockera andra sessioners daemon.
- **`AllowStartOnDemand` = true** krävs för att `schtasks /Run` ska fungera.

**Känd risk (verifieras på hårdvara)**

- `schtasks /Run` på en **grupp-principal-task (implicit interaktiv logon)** ska köra
  daemonen i den inloggades session på medium IL oberoende av installerns
  HIGH IL (Schedulertjänsten skapar processen enligt principalens RunLevel).
  Detta är den punkt som kan bråka på vissa Windows-versioner. Om `/Run` inte
  startar in-session: den icke-fatala "nästa inloggning"-vägen täcker det, och
  ett dokumenterat **`explorer.exe <exe>`-trick** (Explorer kör på medium IL →
  barnet ärver medium IL) finns som nödfallback — **kodas inte nu**.

**Manuellt smoke-test-protokoll (kör på en REN, Webroot-allowlistad maskin)**

Den byggda osignerade exe:n blockeras under Webroot och `go run` kan inte
meningsfullt testa `--install`, så detta är deferrat tills en allowlistad
maskin finns. Steg:

1. Dubbelklicka install-vägen (`prata.exe --install`) → UAC-prompt visas.
2. Godkänn UAC → binär hamnar i `%ProgramFiles%\Prata\prata.exe`.
3. `schtasks /Query /TN Prata /XML` → bekräfta `RunLevel` `LeastPrivilege`,
   `GroupId` `S-1-5-32-545`, LogonTrigger utan `UserId`.
4. Daemonen startad i sessionen → verifiera **medium IL** (Process Explorer:
   Integrity = Medium), inte High.
5. F1 → diktera in i icke-förhöjt Webdoc → text injiceras (UIPI-invarianten
   håller).
6. Kör `--install` igen och avbryt UAC → snygg svensk MessageBox, ingen krasch.

**Diagnostik:** varje installationssteg och fel skrivs tidsstämplat till
`%TEMP%\prata-install.log` (delas av den icke-förhöjda föräldern och det
förhöjda barnet). Läs den om något steg ovan fallerar — den fångar felet även
när installern bara visar en modal MessageBox.

**Verifierat 2026-06-20** på utvecklings-/hem-PC:n (ej Webroot): steg 1–5
bekräftade — binären kopierad till `%ProgramFiles%\Prata`, tasken registrerad,
`RunLevel` = `Limited` (via `Get-ScheduledTask`), daemonen kör från
`%ProgramFiles%\Prata\prata.exe` utan argument, och F1 injicerade i ett oförhöjt
fönster — vilket i sig bevisar medium IL (high IL skulle UIPI-blockera
injiceringen, så steg 4 behöver ingen separat Process Explorer-kontroll).
F1-testet kördes i ett vanligt oförhöjt fönster, inte specifikt Webdoc;
mekanismen är identisk men en explicit Webdoc-bekräftelse återstår. Kvar att
verifiera: UAC-avbryt-rutan (steg 6) och en körning på en ren klinikmaskin.

### 2026-06-21: Särskrivningar i diktering — orsaken är klientens segmentihopslagning, inte whisper-versionen

**Status:** Åtgärdat och live-verifierat. Fixen sitter i
`internal/transcribe/client.go` (`normalizeTranscript`): whisper-segmenten slås
nu ihop **utan** separator, så som Diktell gör, istället för att vartenda radbyte
görs om till ett mellanslag. Användaren har live-verifierat (svensk diktering via
Prata mot **Rngv GPU-server**) att särskrivningarna är borta. `gofmt`, `go vet`
och `go test` är gröna, inklusive ett nytt enhetstest på den verkliga
serveroutputen.

**Bakgrund / symptom**

Diktering via Prata gav mellanslag mitt i svenska ord (särskrivningar) — t.ex.
"tydlighet" → "tyd lighet", "kärnenergifrågan" → "kärnenergifrå gan", "enligt" →
"en ligt". Korta ord var oftast oskadda; det drabbade långa sammansatta ord. Felet
sitter i hur Prata bygger ihop transkriptet — inte i modellen och inte i
whisper-versionen.

**Utredning — förkastad hypotes (whisper-version)**

Första hypotesen var en detokeniserings-regression i whisper.cpp efter taggen
`v1.8.6` (källan på ringvägen stod på `v1.8.6-80-g0ec08451`, 80 commits efter
releasen). Servern byggdes om pinnad till `v1.8.6` och kördes skarpt —
**särskrivningarna kvarstod**. En deterministisk A/B avgjorde saken: samma ljud
(en inspelad WAV) skickades mot både `v1.8.6` och HEAD+80 med
`curl … response_format=verbose_json` och gav **byte-identisk** output, inklusive
exakt samma brytning mitt i ordet. Buggen är alltså versions-oberoende; pinningen
var ett villospår och behövs inte. Runbookens HEAD-klon (PRATA-GPU-SERVER.md) är
därför oförändrad.

**Diagnos — verklig orsak**

`verbose_json` på riktig svensk röst visade mekanismen. whisper lägger ibland en
**segmentgräns mitt i ett långt ord**:

```
"text": " … kärnenergifrågan. Tyd\nlighet, små, enligt, akromeoplastik.\n"
segment 0 slutar: "… kärnenergifrågan. Tyd"   (inget avslutande mellanslag)
segment 1 börjar: "lighet, små, enligt, …"     (inget inledande mellanslag)
```

Servern serialiserar varje segment på egen rad i `text`-fältet (`"Tyd\nlighet"`).
Pratas `normalizeTranscript` körde `strings.Join(strings.Fields(s), " ")`, vilket
behandlar radbytet som vilket blanktecken som helst och gör om det till ett
mellanslag → "Tyd lighet". Vid en **riktig** ordgräns bär nästa segment sitt eget
inledande mellanslag (jfk: `"… country can\n do for you …"`), men vid en gräns
**inuti** ett ord saknas det. Diktell drabbas inte eftersom det konkatenerar
segmenten utan separator — exakt det `normalizeTranscript` påstod sig göra men
inte gjorde.

**Beslut**

Ta bort segmentens radbyten utan att ersätta dem med mellanslag (konkatenera, som
Diktell), och kollapsa sedan kvarvarande blanktecken. En riktig ordgräns har redan
sitt inledande mellanslag på nästa segment; en gräns mitt i ordet har det inte — så
att droppa radbytet ger rätt resultat i båda fallen. Ett enhetstest kör den exakt
inspelade serveroutputen och kräver "Tydlighet", inte "Tyd lighet".

**Alternativ övervägda**

- **Pinna/bygg om whisper.cpp till `v1.8.6`** (ursprungshypotesen). Förkastad:
  A/B visar att buggen är versions-oberoende, så pinningen ändrar ingenting.
- **Patcha whisper.cpp-servern** (sluta bryta mitt i ord / sluta infoga radbyte).
  Förkastad: fork-underhåll, och ihopslagningen hör hemma på klienten — Diktell
  bevisar att klient-konkatenering är rätt.
- **Korrigera i Pratas ordlista.** Förkastad: maskerar felet och generaliserar
  inte (godtyckliga ord drabbas).

**Konsekvenser**

- Fixen är en liten ändring i `normalizeTranscript` + tester; verifierad lokalt och
  i skarp diktering.
- whisper.cpp-serverns version är irrelevant för denna bugg; ingen pinning behövs
  och inget i PRATA-GPU-SERVER.md ändras. Den v1.8.6-ombyggnad som gjordes under
  utredningen är ofarlig men onödig.
- Gäller alla backends som serialiserar segment per rad (whisper.cpp-servern och
  Berget lika).
- Stavningsvariationen io→eo ("akromeoplastik") är ett ASR-igenkänningsfel från
  modellen och påverkas inte av denna fix.
- Släppt i **v0.3.0** (omdistribution via `prata.exe --install` / `Installera-Prata.bat`).