# Prata — Komplett översikt för extern granskning

> **Syfte.** Detta är ett *självbärande* dokument avsett att klistras in i olika
> AI-modeller för att få synpunkter, kritik och nya idéer. En granskare ska kunna
> förstå hela appen — funktioner, teknik, designval och öppna frågor — utan
> tillgång till koden eller övriga dokument.
>
> **Status.** Ögonblicksbild **2026-06-21** (efter v0.3.0; några finputsningar
> ligger i `[Unreleased]`). Detta dokument är en *destillering* — den löpande
> sanningen finns i `PRATA-MASTER.md`, `PRATA-DESIGN-LOG.md`,
> `PRATA-GPU-SERVER.md`, `README.md` och `CHANGELOG.md`. Det genereras inte
> automatiskt; uppdatera det när du vill ha en ny granskningsrunda.
>
> **Längst ned** finns en sektion *"Frågor till granskaren"* — börja gärna där om
> du är en AI som ombeds ge feedback.

---

## TL;DR

Prata är en minimal, Windows-native push-to-talk-app för **svensk medicinsk
diktering**. Du håller **F1**, talar, släpper — ljudet transkriberas med
`KBLab/kb-whisper-large` mot en vald backend (lokal whisper.cpp-GPU-server över
nätet, eller Berget Ai i molnet), körs genom en korrigeringsordlista och skrivs in
i fönstret som var aktivt när du tryckte F1. En andra operation, **F8**, är en
snabbfix för ordlistan. Appen har inget eget fönster — bara en tray-ikon. Den är
skriven i **Go** med **ett enda externt beroende** (`malgo` för ljud); allt annat
är direkt Win32 via `syscall`. Den är byggd för att *installeras och glömmas*
("see and forget") på ~12 delade klinikdatorer, och hela designen är genomsyrad av
**patientsekretess** (ljud aldrig till disk, dikterad journaltext aldrig till
urklipp/molnurklipp).

Prata är ett systerverktyg till **Diktell** (samma utvecklares befintliga,
frysta dikteringsapp i Rust med lokal CUDA-Whisper). Diktell kräver en dedikerad
GPU; Prata fyller luckan på maskiner utan GPU.

---

## 1. Kontext och problem

- **Användaren** är ortoped/läkare som bygger AI-verktyg men inte skriver kod
  själv — all kod drivs via AI-assistenter. Hög arkitektonisk förståelse, läser
  kod på hög nivå.
- **Miljön** är ett sjukhus där användaren ofta **byter dator under dagen**.
  Många av dessa är mini-PC:s **utan GPU**, där Diktell (lokal CUDA-Whisper) inte
  kan köras. Det är problemet Prata löser.
- **Texten landar i en patientjournal** (webbaserad, "Webdoc"). Det höjer ribban:
  fel injektion är en patientsäkerhetsrisk, och patientdata får inte läcka.
- **Skala:** ~10–12 klinikdatorer, inloggade kliniker har lokal admin,
  distribution via USB-minne (inte Intune/SCCM ännu, men designen är förberedd för
  det).

---

## 2. Designprinciper

1. **"See and forget"** — installeras en gång, ska fungera i åratal utan tillsyn.
   Driver valet av Go (Go 1 compatibility promise), en självständig binär utan
   runtime, och inga konfigurationsfiler.
2. **Minimalism / stdlib-only** — ett enda externt beroende (`malgo`). Allt annat
   (HTTP, DPAPI, urklipp, hotkeys, ljud, tray, installer) är direkt Win32
   P/Invoke. Inga paketeringsverktyg (MSI/Inno/WiX), inga ramverk.
3. **Patientsäkerhet är en hård invariant** — flera designval (se §5, §8, §9) är
   låsta för att dikterad text aldrig ska hamna fel eller läcka.
4. **Windows-only just nu** — ingen cross-platform-abstraktion betalas innan den
   behövs. Mac/Linux är "kanske senare".
5. **Inget app-initierat arbetsflödes-UI** — bara ljudsignaler i flödet, en passiv
   tray-ikon, och en användarinitierad F8-popup. Appen "stör" aldrig.

---

## 3. Funktioner

### 3.1 F1 — diktering (huvudflödet)

1. Användaren håller **F1** nere (global hotkey via `RegisterHotKey`).
2. Prata fångar förgrundsfönstret (mål för injektion) och spelar in mikrofonen
   (16 kHz mono PCM via WASAPI/`malgo`). En startton (880 Hz) spelas.
3. Vid släpp (stoppton 587 Hz): PCM → WAV → POST (multipart, OpenAI-kompatibelt)
   till vald backend.
4. Svaret normaliseras till löpande prosa (segmentihopslagning — se §7.4) och
   körs genom korrigeringsordlistan.
5. Målfönstret återställs och texten skrivs in via **klassbaserad routing** (se
   §8). Kan fönstret inte återställas säkert → injektionen avbryts med felton
   (hellre ingen text än fel ställe).
6. Transkribering sker **asynkront i en FIFO-worker** — en långsam backend-runda
   blockerar inte nästa F1-inspelning.

### 3.2 F8 — snabbfix för ordlistan

1. Användaren markerar ett feltranskriberat ord/uttryck och trycker **F8**.
2. Prata kopierar markeringen och visar en liten popup (DWM-skugga, rundade hörn,
   F8-chip) förankrad över markeringen.
3. Användaren skriver rätt form, trycker Enter (Esc/klick utanför avbryter).
4. Regeln sparas i per-användarens override-fil, ordlistan laddas om, källfönstret
   återställs och den korrigerade texten klistras tillbaka.

F8- och F1-injektioner är **serialiserade** så att deras urklipps-/tangent-
operationer inte kan flätas in i varandra.

### 3.3 Övrigt

- **Backend-väljare** i tray-menyn (radioknappar) — se §7.
- **Ljudsignaler** syntetiseras i processen (winmm `PlaySoundW`), inga ljudfiler:
  start (hög ton), stopp (låg ton), fel (dubbel låg puls på alla tysta felvägar).
- **Tray-ikon** (liten röd Prata-ikon): backend-val, "Sök efter uppdatering…",
  "Avsluta". Primärt sätt att avsluta när appen kör vid inloggning utan konsol.
- **Uppdateringskoll** — notifierande, aldrig självuppdaterande (se §9.3).
- **Single-instance-vakt** — namngiven, sessionsbunden mutex (`Local\`) → en
  instans per session på en delad PC.
- **Autostart** via en maskinbred Task Scheduler-uppgift (se §9).

---

## 4. Arkitektur och teknik

| Område | Val | Not |
|---|---|---|
| Språk | **Go** (1.26, se `go.mod`) | Go 1 compat, självständig binär, ~150 MB toolchain. |
| Ljudfångst | **gen2brain/malgo** (miniaudio/WASAPI, cgo) | Enda externa beroendet; `CGO_ENABLED=1`. |
| Hotkeys, tray, urklipp, injektion, DPAPI, popup, MessageBox, single-instance, installer | **stdlib `syscall` + direkt Win32** | Inga tredjepartsbibliotek; bindningarna är handskrivna i `internal/`. |
| HTTP | **stdlib `net/http`** | multipart POST till OpenAI-kompatibla transkriberingsendpoints. |

**Trådmodell:** hotkey-lyssnaren kör på meddelandekön (`RegisterHotKey` → `WM_HOTKEY`
för tryck; 20 ms `GetAsyncKeyState`-polling för släpp, startad vid tryck och avslutad
vid släpp = noll vilokostnad). Transkriberingen körs i **en FIFO-worker** skild från
inspelningen.

**Paketkarta (`internal/`):** `audio` (malgo-capture), `transcribe` (multi-backend
HTTP-klient + WAV-encoder + normalisering), `hotkey` (F1/F8 via RegisterHotKey),
`inject` (hybrid textinjektion), `dict` (ordlista: embeddad baslinje + override),
`sanity` (degenererings-vakt via gzip-ratio), `auth` (DPAPI), `single`
(mutex-vakt), `cue` (ljudsignaler), `tray` (ikon/meny/balong/uppdateringskoll),
`icon` (`go:embed` av ikonen), `installer` (maskinbred `--install`/`--uninstall`),
`ui` (`MessageBox`-helper), `update` (notifierande versionskoll), `popup`
(F8-popupen, Win32/DWM). `cmd/prata/` är daemonen + subkommandona; `cmd/*-test/`
är isolerade smoke-test-/kalibreringsverktyg.

---

## 5. Transkribering och backends

### 5.1 De tre backendarna

| Visningsnamn | Stabilt ID | Pekar på | Auth |
|---|---|---|---|
| Rngv GPU-server (Tailscale) | `Hemma` | Hem-GPU (whisper.cpp) över Tailscale | Ingen |
| LAN GPU-server | `Jobb` | Klinikens GPU på LAN | Ingen |
| Berget Ai | `Berget` | Berget Ai moln-API | Bearer-nyckel (DPAPI) |

- Alla kör **samma modell** (`KBLab/kb-whisper-large`) → samma felmönster, och
  Diktells `dictionary-corrections.txt` är direkt återanvändbar.
- **Endpoint-URL:er är hårdkodade konstanter** i binären (backend-*valet* är
  tillstånd, inte konfiguration).
- **Berget:** `https://api.berget.ai/v1/audio/transcriptions`, multipart,
  zero retention, servrar i Stockholm (data lämnar inte Sverige — för en läkare
  förmodligen den enda *legitima* molntjänsten för dikterad medicinsk text).
  ~50 öre/månad vid användarens volym; ~2,6 s latens (mätt).

### 5.2 Val, persistens, default

- Valet sparas som **stabilt ID** i `%LOCALAPPDATA%\Prata\backend.txt` →
  visningsnamn kan ändras utan att bryta sparade val.
- **Default vid första körning: `Jobb` (LAN GPU-server)** — intern GPU utan
  nyckel. (Annars hade en ny användare träffat Berget-utan-nyckel vid F1 → felton.)
- **Ingen tyst failover** — är vald server nere får du felton, inte ett tyst byte.
  Byte sker bara när användaren väljer i menyn.

### 5.3 Nätverkstopologi och sekretess

- **Klinikens GPU exponeras ALDRIG över Tailscale.** Patientljud får inte lämna
  klinikens nät. Brandväggen scopas till LocalSubnet/Domain.
- **Hem-GPU** nås alltid externt över Tailscale (Tailscale-IP, CGNAT-intervall
  `100.64.0.0/10`). Egna maskiner, inte patientljud.
- GPU-servern körs som en egen SYSTEM-task (`PrataWhisperServer`) — skild från
  Prata-klientens task.

### 5.4 Textnormalisering (en lärorik bugg)

whisper-servrar serialiserar varje tidssegment på egen rad i `text`-fältet.
whisper lägger **ibland en segmentgräns mitt i ett långt ord**. Det avgör hur
raderna ska slås ihop — och det skiljer sig per backend:

- **Lokal whisper.cpp** lämnar segmenttexten **otrimmad**: en äkta ordgräns bär
  sitt eget inledande mellanslag på nästa segment; bara en gräns *inuti* ett ord
  saknar det. Därför: **droppa radbytet utan separator** → "Tyd"+"lighet" =
  "Tydlighet" (rätt), och äkta ordgränser behåller sitt mellanslag.
- **Berget** **trimmar** varje segmentrad → radbytet är då det *enda* som skiljer
  mening från mening. Därför: **låt radbytet bli ett mellanslag** → annars
  "förluster.Ungdomarna".

Lösningen är en `Backend.TrimmedSegments`-flagga (true endast för Berget).
Heuristik på skiljetecken förkastades: "få"+"skriva" (ska separeras) och
"Tyd"+"lighet" (ska limmas) är båda bokstav+bokstav utan mellanslag — omöjliga att
skilja åt utan tokendata.

### 5.5 Degenererings-vakt (`internal/sanity`)

whisper kan fastna i repetitionsloopar (samma fras om och om igen). Prata mäter
**gzip-kompressionsgrad** på utdata och kasserar degenererad output innan den
skrivs in. Tröskeln kalibreras med `cmd/sanity-test`.

---

## 6. Korrigeringsordlistan

Två lager:

1. **Baslinje** — `go:embed`:ad i binären vid build
   (`internal/dict/dictionary-corrections.txt`). Laddas alltid → kan inte "tyst
   inaktiveras" för att en fil saknas.
2. **Per-användare-override** — `%LOCALAPPDATA%\Prata\dictionary-corrections.txt`
   (skapas vid första F8-spar). Override **lägger till** eller **ersätter per
   nyckel** baslinjeposter (first-match-wins).

- Matchning är skiftlägeskänslig med **Unicode-medvetna ordgränser**
  (`[\p{L}\p{N}_]`), literal indexering (ingen regexp), regler i filordning.
- **Byggtids-fold-in (designat, implementation faslagd):** ett litet CLI
  (`cmd/dict-foldin`) ska kunna vika in värdefulla override-poster i den embeddade
  baslinjen inför en release (klinikkorrigeringar = domänkunskap, inte personlig
  preferens). Kontraktet är specat; verktyget är inte byggt ännu.

---

## 7. Textinjektion (klassbaserad routing)

Detta är ett av de mest säkerhetskänsliga besluten.

- **Routing på förgrundsfönstrets klass** (`GetClassNameW(GetForegroundWindow())`):
  - `Chrome_WidgetWin_1` (hela Chromium/Electron-familjen + den webbaserade
    journalen, bekräftat samma klass) → **SendInput Unicode**, hela strängen i
    *ett* anrop. Urklippet rörs aldrig.
  - Alla andra fönster → **urklipps-paste** (`CF_UNICODETEXT`, spara/återställ
    föregående urklipp).
- **Invarianter (patientsäkerhet — får inte ändras):**
  - **Säker default:** all osäkerhet (inget förgrundsfönster, misslyckad
    klassläsning, okänd klass) → urklipps-paste.
  - **Ingen exekverings-fallback:** vägen väljs en gång. Vid SendInput-fel faller
    den *aldrig* tillbaka på paste — SendInput kan redan ha skickat tecken, och en
    efterföljande paste skulle dubbelinjicera (i en journal en säkerhetsrisk).
    Tappad text → användaren omdikterar (säkert).
  - **Allowlista, inte denylista:** otestade appar defaultar till den bevisade
    paste-vägen. Inget får SendInput förrän klassen verifierats med realistisk,
    flerradig text. **Exakt** klassmatchning, inte prefix.
- **Varför:** (1) i AI-chattar ska man kunna kopiera en skärmbild, diktera, och
  sedan Ctrl+V:a in bilden — dikteringen får inte röra urklippet; (2)
  patientsekretess: journaltext ska inte ligga kvar i Win+V eller synka till
  molnurklippet.
- **Historik:** en tidig Unicode-väg tappade key-up-event i Chromium/moderna
  Notepad → OS-autorepeat. Det löstes genom att skicka hela transkriptionen i
  *ett* SendInput-anrop. Moderna Notepad allowlistas medvetet *inte* (SendInput
  fallerar där längd-/innehållsberoende).

---

## 8. Hotkeys

- **F1 = PTT-diktering, F8 = ordlistesnabbfix.** Via `RegisterHotKey`
  (`MOD_NOREPEAT`), inte en `WH_KEYBOARD_LL`-hook.
- **Varför inte hook:** lågnivå-tangentbordshookar har en dokumenterad felklass
  (tyst avinstallation vid >~300 ms callback, ogiltigförklaring vid sleep/resume,
  och AV/EDR-misstanke — hookar mönstermatchar keyloggers). Diktell bär den
  klassen med en watchdog; Prata vill inte ärva den. En kanary (`cmd/regkey-test`)
  bevisade att bara F-tangenter via `RegisterHotKey` *inte* når den fokuserade
  appen (en tidigare motobservation visade sig vara en crate-artefakt).
- **F8 (inte F9):** Diktell äger F9 (och Ctrl+Win). Genom att Prata tar F8 kan
  båda apparna köras parallellt på samma maskin: **F9 = Diktell, F8 = Prata
  snabbfix, F1 = Prata PTT**. Det möjliggör också A/B-jämförelse av de två
  pipelinerna på samma diktering.
- F1:s nativa Hjälp-funktion konsumeras systemvitt medan Prata kör; återställs vid
  exit.

---

## 9. Distribution och livscykel

### 9.1 En binär, maskinbred install

- Leveransen är **en enda `prata.exe`** + USB-wrappers `Installera-Prata.bat` /
  `Avinstallera-Prata.bat`. Samma binär kör daemon, `--install`, `--uninstall`,
  `--set-key`.
- `prata.exe --install` (self-elevating via UAC): kopierar binären till
  `%ProgramFiles%\Prata\` (skrivskyddad för icke-admin → daemonen kan inte
  modifiera sin egen image), registrerar en **maskinbred Task Scheduler-uppgift**
  (`Prata`, alla användare via SID `S-1-5-32-545`), och startar i sessionen via
  `schtasks /Run`.
- **All skrivbar state är per-användare** i `%LOCALAPPDATA%\Prata\` (`apikey.dat`,
  `backend.txt`, ordlista-override). **Ingen maskinbred skrivbar data** → inget
  `%ProgramData%`, inga ACL-/multisession-write-race.
- `--uninstall` stoppar daemonen, tar bort tasken + `%ProgramFiles%\Prata`, men
  **lämnar per-användardata** (dyr att återskapa; symmetri — install skapade den
  aldrig).

### 9.2 Den hårda elevations-invarianten (UIPI)

Daemonen kör på **medium IL** (Task Scheduler RunLevel Limited). **Bara**
install-åtgärden förhöjer. En förhöjd daemon skulle **tyst** bryta
SendInput-injektion i ett icke-förhöjt Webdoc (UIPI blockerar lågnivå-input från
high IL → medium IL). Därför startas daemonen aldrig direkt från den förhöjda
installern; post-install-start sker via `schtasks /Run` (medium IL).

### 9.3 Uppdatering — notifierande, inte självuppdaterande

- Binären stämplas med version via `-ldflags "-X main.version=…"`.
  `internal/update.Check` frågar GitHubs latest-release-API och jämför `vX.Y.Z`.
  Tray-item "Sök efter uppdatering…" rapporterar i en balong.
- **Uppgraderingen är manuell:** kör om `--install` från den *nya* binären på USB.
  En `samePath`-vakt gör att den redan installerade binären bara reparerar tasken
  (ingen versionshöjning) — uppdatering måste ske från USB-kopian.
- **Varför inte självuppdatering:** en binär som laddar ner och kör en ersättning
  av sig själv är precis det download-and-execute-mönster som beteende-AV/EDR
  flaggar för en osignerad exe (se §10). Det skulle dessutom lägga en tyst
  felväg i den enda operation som inte får gå fel på ett kliniskt verktyg.

---

## 10. Det stora öppna problemet: osignerad binär vs AV/EDR

- En osignerad, nybyggd `prata.exe` blockeras vid start av beteende-AV (bekräftat:
  **Webroot SecureAnywhere**). Symptom: loader-avvisning ("not a valid Win32
  application" / "Åtkomst nekad"), ingen krasch loggad. Orsak: en okänd,
  nollprevalens-binär som registrerar hotkeys, fångar mikrofonen och syntetiserar
  tangenttryck = lärobokens "misstänkta okända".
- **`go run` fungerar** (kör från Go-byggcachen, som Webroot tolererar) → det är
  den verifierade dev-vägen.
- **Nuvarande hantering (skala ~12 maskiner):** USB-kopierade exe:er saknar
  Mark-of-the-Web → SmartScreen triggar inte; **per-maskin-allowlisting** ersätter
  publik signering. Windows Defender-undantag sätts av `--install` själv
  (`Add-MpPreference`); tredjeparts-EDR allowlistas i konsolen (dokumenterat i
  USB-runbooken).
- **Den varaktiga fixen — Authenticode-signering — är förberedd men deferrad:** en
  no-op hook i `release.yml` (gated på `CODE_SIGN_PFX`-secret) tills ett cert finns.
  Publikt EV-cert blir kritiskt först vid IT-driven skalning (Intune/SCCM).

---

## 11. Säkerhet och integritet (sammanfattning)

- **Patientljud skrivs aldrig till disk** — buffras i minnet, kasseras efter
  transkriberingsrundan.
- **Dikterad journaltext lämnar aldrig urklippet** i Chromium/journalen (SendInput-
  vägen) → varken Win+V eller molnurklipp.
- **Berget-nyckeln är DPAPI-krypterad** per användare/maskin
  (`%LOCALAPPDATA%\Prata\apikey.dat`) — oläsbar för andra konton/maskiner. *Ingen*
  machine-scope DPAPI (skulle exponera nyckeln för alla på en delad PC).
- **Klinikens GPU exponeras aldrig över Tailscale.**
- Repot är **privat**.

---

## 12. Viktiga designbeslut i korthet (med motivering)

| Beslut | Motivering | Förkastat alternativ |
|---|---|---|
| Go, inte Rust | "See and forget", stdlib täcker det mesta, liten toolchain, självständig binär | Rust (samma stack som Diktell, men tyngre toolchain) |
| Ett externt beroende (`malgo`) | Minimalism, långsiktig stabilitet | Bibliotek för tray/hotkey/urklipp (valdes bort till förmån för Win32 direkt) |
| Hårdkodade endpoints, inget config | "See and forget" | `config.toml` |
| F1/F8 via `RegisterHotKey` | Undviker hook-failklassen + AV-misstanke | `WH_KEYBOARD_LL` (Diktells väg) |
| Klassbaserad hybridinjektion | Patientsekretess + robusthet i Chromium/journal | Ovillkorligt SendInput (bröt Notepad/journal); denylista (osäker default) |
| Backend-specifik segmentihopslagning | Lokal whisper otrimmad, Berget trimmad | Skiljetecken-heuristik (omöjlig utan tokendata) |
| Notifierande uppdatering | Självuppdatering = AV-flaggat download-and-execute | Full self-update; tyst auto-koll vid start (deferrad) |
| Maskinbred install, per-användare state | Delade PC, byter dator; ingen maskinbred skrivbar data | `%ProgramData%`-delad ordlista (ACL/race); MSI/Inno/WiX (bryter en-fil) |
| Medium IL via Task Scheduler | UIPI: förhöjd daemon bryter injektion tyst | HKLM\Run (ingen RunLevel-kontroll) |
| Signering deferrad | USB + allowlisting räcker vid ~12 maskiner | Att blockera all leverans på EV-cert-ledtid |

---

## 13. Vad som fungerar bra (verifierat)

- **Fas 0-validering (2026-05-27):** Berget ger *identiskt felmönster* som lokal
  Diktell (samma modell); Diktells ordlista direkt återanvändbar; latens medel
  2,61 s (min 2,56 / max 2,77) över 5 anrop, ingen kallstartseffekt.
- **Skarp diktering live-verifierad** på sekundär maskin (4 dikteringar,
  ~2,1–2,7 s round-trip).
- **Hybridinjektion** verifierad ren i Chrome, Cursor, Claude Desktop (flerradig
  text) och i journalsystemet via `cmd/inject-test` (klass bekräftad).
- **Maskinbred install/uninstall/uppdatering hårdvaruverifierad (2026-06-20):**
  överskriv-medan-igång (döda gammal daemon → retry-copy → omregistrering →
  omstart), medium-IL-injektion i oförhöjt fönster, användardata bevarad.
- **Särskrivnings-/Berget-spacing-buggarna** lösta och live-verifierade, med
  enhetstester på verklig serveroutput.
- **F8-popupen** omstylad (DWM-skugga, rundade hörn, centrerad text) och
  live-verifierad.

---

## 14. Medvetna begränsningar och icke-mål

- **Inte cross-platform** (Windows-only; Mac/Linux "kanske senare").
- **Inte konfigurerbar** — ändra konstant + kompilera om.
- **Inte kommersiell** — personligt + kollegialt bruk.
- **Inte moln-först** — local-first med moln-fallback (Berget) för transkribering.
- **Inget ramverk** — ett verktyg.
- **Ingen AI-efterbearbetning** av texten (till skillnad från vissa
  dikteringsverktyg) — bara deterministisk ordlistekorrigering.

---

## 15. Frågor till granskaren (öppna trådar)

Om du är en AI som ombeds ge synpunkter: här är de punkter där feedback och idéer
är mest värdefulla.

1. **Signering / leverans.** Är "USB + per-maskin-allowlisting + deferrad
   signtool-hook" rätt avvägning vid ~12 maskiner? Vilken signeringsväg (OV/EV,
   Azure Trusted Signing, internt cert i EDR) ger bäst nytta/kostnad innan
   IT-driven skalning? Finns ett sätt att minska AV-friktionen *utan* cert?
2. **Injektionens täckning.** Klassbaserad allowlista med exakt
   `Chrome_WidgetWin_1`. Vilka realistiska fönsterklasser (Win32-native journaler,
   Java/Qt-appar, RDP/Citrix-sessioner, virtuella desktops) riskerar att hamna på
   den säkra-men-urklippsläckande paste-vägen, och hur skulle du verifiera nya
   klasser säkert? Är "ingen exekverings-fallback" rätt även när SendInput
   *garanterat* inte hann skicka något?
3. **Patientsekretess i paste-vägen.** På icke-Chromium-fönster används urklipp
   (spara/återställ). Det finns ett kort tidsfönster där journaltext ligger i
   urklippet. Hur allvarligt är det, och finns en bättre väg som inte bryter
   en-fil-/stdlib-only-principen?
4. **Multisession på delad PC.** `--install`/uppdatering dödar *alla* andras
   `prata.exe`. Är "uppdatera när ingen dikterar" en hållbar driftsregel, eller bör
   uppdateringen vara sessionsmedveten?
5. **Ordlista-fold-in.** Gränssnittet (`cmd/dict-foldin`) är specat men inte byggt.
   Är manuell fold-in inför release rätt, eller bör klinikkorrigeringar
   synkroniseras på något smartare sätt mellan ~12 maskiner?
6. **Backend-robusthet.** Ingen tyst failover (medvetet). Men vore en *explicit*,
   användarbekräftad fallback (t.ex. "LAN nere — vill du använda Berget?") värd
   komplexiteten? Hur påverkar det sekretessmodellen?
7. **Degenererings-vakten.** gzip-ratio-tröskel mot whisper-repetitionsloopar — är
   det robust, eller finns falska positiv/negativ-risker för korta kliniska fraser?
8. **Ergonomi.** F1 (PTT) + F8 (snabbfix). Risk för Fn-lager på mini-PC-tangentbord
   (kräver Fn+F1). Bättre tangentval, eller är detta rätt?
9. **Generella idéer.** Vad saknas för att detta ska vara ett robust kliniskt
   verktyg i åratal utan tillsyn? Vilka felmoder har vi inte tänkt på?

---

## 16. Teknisk fakta-appendix

- **Modell:** `KBLab/kb-whisper-large` (GGUF, lokalt på GPU; samma via Berget).
- **Berget-endpoint:** `https://api.berget.ai/v1/audio/transcriptions`.
- **Hem-GPU-endpoint (exempel):** `http://100.87.6.56:8080/v1/audio/transcriptions`
  (Tailscale-IP).
- **Ljud:** 16 kHz mono PCM, WASAPI via `malgo`.
- **Hotkeys:** F1 (PTT), F8 (ordlistesnabbfix), via `RegisterHotKey` + `MOD_NOREPEAT`.
- **Latens:** ~2,6 s medel mot Berget (mätt); lokal GPU snabbare vid upprepad
  diktering, ~1,85 s modelladdning vid lokal kallstart.
- **Per-användare-sökvägar:** `%LOCALAPPDATA%\Prata\{apikey.dat, backend.txt,
  dictionary-corrections.txt}`.
- **Installationsväg:** `%ProgramFiles%\Prata\prata.exe` (skrivskyddad), Task
  Scheduler-uppgift `Prata` (medium IL, alla användare).
- **Bygg:** `go build -ldflags="-s -w -H windowsgui -X main.version=<tag>" -o
  prata.exe ./cmd/prata/`, `CGO_ENABLED=1`.
- **CI:** `gofmt -l .` → `go vet` → `go build` → `go test` på `windows-latest`.
- **Beroende:** `github.com/gen2brain/malgo` (enda externa).
