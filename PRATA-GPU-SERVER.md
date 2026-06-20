# Prata — GPU-server för LAN-transkribering

> **Status:** utkast under uppbyggnad. Fylls på steg för steg allt eftersom
> varje steg verifieras på riktig hårdvara.
> **Senast uppdaterad:** 2026-06-16

## Syfte

Den här guiden beskriver hur man reser en lokal KB-Whisper-server på en maskin
med ett RTX 50-kort (Blackwell), så att Prata kan diktera mot den över
nätverket i stället för mot Berget AI. Skriven för att kunna reproduceras på en
ny maskin i framtiden.

Servern kör samma modell som Diktell (KB-Whisper-large GGML) men exponerar den
som en OpenAI-kompatibel HTTP-endpoint, vilket är exakt formen Prata redan
pratar.

## Arkitektur och principer

Kort om varför uppsättningen ser ut som den gör — det styr alla val nedan.

**En GPU-server per plats, lokal i sitt eget nät.** Varje plats (hemma, jobbet)
har sin egen server på sitt eget nät. De två världarna — hem och jobb — bryggas
aldrig ihop. På jobbet sitter klient och server alltid i samma fysiska nät.

**Hemma tillåts fjärråtkomst inom det egna tailnet:t.** Hemmaservern får nås
från en annan av dina *egna* maskiner (t.ex. Landet-PC:n) över Tailscale. Det
är inte en bryggning mellan hem och jobb — det är din privata mesh mellan dina
egna enheter, och hemmabruket gäller privat diktering, inte klinisk
patientljud. Jobbservern exponeras *aldrig* över Tailscale (se Steg 4).

**Patientljud lämnar aldrig nätet.** På jobbet stannar all ljuddata inom
klinikens/regionens nät — klient → server är intern trafik. Att skicka klinisk
ljuddata ut till en privat maskin är uteslutet (GDPR/informationssäkerhet),
samma logik som gjorde att Berget valdes framför ElevenLabs/Azure.

**Ingen automatisk failover.** Prata växlar aldrig backend tyst. Aktiv backend
väljs medvetet och visas alltid. Är aktiv server nere → felton, inte tyst byte.
En backend som byts under dig i ett injektionsverktyg för patientdata är ett
patientsäkerhetsproblem, inte en bekvämlighet.

**Återanvänder Diktells modell och byggkedja.** whisper.cpp valdes som server
(inte faster-whisper) av två skäl: (1) den laddar exakt samma `ggml-model.bin`
som Diktell redan använder — byte-identiskt modellbeteende, ingen
formatkonvertering; (2) den återanvänder den CUDA-byggkedja du redan har på
plats för Diktell.

## Verifierad topologi (2026-06-16)

Tabellen beskriver den faktiska driftsättningen efter att rum 4 (klient) skulle
nå GPU-servern i rum 1. Använd den som referens vid felsökning på nya maskiner.

| Maskin (Tailscale-namn) | LAN-IP | Tailscale-IP | Roll | Nyckelfakta |
|---|---|---|---|---|
| **rum-ett** | `10.64.3.60` (statisk) | `100.80.217.12` | Jobb-PC, **GPU — server** | `whisper-server` port 8080, autostart vid boot |
| **rum4-9700k** | `10.64.3.59` | `100.78.209.16` | Jobb-PC, **utan GPU — klient** | Kör Prata, backend Rum1 GPU-server |
| **ringvagen** | — | `100.87.6.56` | Hem-PC (Windows 11), GPU — server | Backend Rngv GPU-server, nås via Tailscale |
| **ringvagen-wsl** | — | `100.115.64.39` | WSL på hem-PC | Cursor SSH, **ej** i transkriberingsvägen |

**Vald produktionsväg i kliniken:** rum4 → rum-ett **direkt över LAN**
(`10.64.3.59 → 10.64.3.60`, samma subnät, Ethernet). Patientljudet stannar
därmed inom sjukhuset — inget Tailscale-relä inblandat. Tailscale behålls som
fallback och för fjärråtkomst hemma.

**Tailscale är ett mesh (tailnet), inte parvisa uppkopplingar.** Varje maskin
som loggats in med samma konto blir en nod och når alla andra via sin `100.x`-IP.
Man "kopplar inte upp mot" en enskild maskin.

**Fallback om LAN-vägen stängs** (segmentering): peka Jobb-backenden till rum-etts
Tailscale-IP `100.80.217.12` i stället för LAN-IP. Kontrollera då med
`tailscale status` att kopplingen står som **direct** (inte **relay**) — relay
innebär att krypterad trafik går ut via ett DERP-relä utanför huset. Detta är
en nödfallback, inte produktionsvägen (patientljud ska stanna på LAN).

**Känd varning (MagicDNS):** `tailscale status` kan rapportera att MagicDNS inte
kunde sätta DNS-konfigurationen ("filen används av en annan process"). Konsekvens:
använd `100.x`-IP-adresser, inte värdnamn, tills det är löst. Blockerar inte
IP-trafiken.

**Mätt latens (2026-06-16, rum-ett GPU):** ca **1,4 s** per diktering. Jämför
Berget Ai ~2,6 s (mätt 2026-05-27).

### Två separata komponenter — viktigt att hålla isär

Servern består av **två delar** som hanteras på olika sätt:

| Komponent | Vad det är | Hur det hamnar på maskinen |
|---|---|---|
| **whisper.cpp** | Serverprogrammet — open source C++ som tar emot HTTP-anrop och kör inferens | Byggs från källkod i Steg 1 (en gång) |
| **KB-Whisper-large** (`ggml-model.bin`) | Modellen — en färdigtränad fil från KBLab. Det är den som avgör att du får svenska och inte generisk Whisper | Laddas ned (Alternativ B) eller finns redan via Diktell (Alternativ A) |

Du "bygger" alltså **inte** KB-Whisper — den är färdigtränad av KBLab och laddas
ned som en enstaka ~3 GB-fil. Det är `-m`-flaggan i Steg 2 som pekar
serverprogrammet på just KB-Whisper-filen. Den garantin sitter i det kommandot,
inte i bygget.

---

### Alternativ A — Diktell redan installerat på maskinen

**Byggkedja** (CUDA Toolkit, CMake, Visual Studio Build Tools 2022) finns redan
på plats. Inget extra att installera.

**KB-Whisper-modellen** finns redan — det är *exakt* samma `ggml-model.bin` som
Diktell laddar. Hitta sökvägen:

```powershell
Get-Content C:\Dev\diktell\config.toml | Select-String "model_path"
# typiskt: model_path = "models/ggml-model.bin"
# dvs C:\Dev\diktell\models\ggml-model.bin
```

Du kan peka servern direkt på Diktells modell, eller kopiera filen till en egen
katalog. På den här maskinen finns en kopia i
`C:\Dev\whisper-models\ggml-model.bin` — det är den path autostart-uppgiften och
Steg 2-kommandot använder. Byt path om du väljer en annan plats.

**→ Gå direkt till Steg 1.** Det enda som saknas är serverprogrammet whisper.cpp.

---

### Alternativ B — ny maskin utan Diktell

Server-maskinen behöver:

- Ett RTX 50-kort (Blackwell, compute capability sm_120). Gäller både 5070 Ti
  och 5060 Ti.
- NVIDIA-drivrutin 570+ och CUDA Toolkit 12.8 eller senare (Blackwell kräver
  minst 12.8).
- CMake 3.20+, Visual Studio Build Tools 2022 med C++-arbetsbörda, Git.

Se Diktells `docs/03-dev-environment.md` för CUDA/CMake/Build
Tools-installationen (~30 min).

**Hämta KB-Whisper-large** från KBLab (~3 GB — ta en kopp kaffe):

```powershell
New-Item -ItemType Directory -Path C:\Dev\whisper-models -Force
Invoke-WebRequest `
  -Uri "https://huggingface.co/KBLab/kb-whisper-large/resolve/main/ggml-model.bin" `
  -OutFile "C:\Dev\whisper-models\ggml-model.bin"
```

KB-Whisper-large är en svensk Whisper-variant tränad av KBLab på svenska texter.
Generisk Whisper ger sämre resultat på svenska och används inte.

**→ Fortsätt till Steg 1.**

## Steg 1 — Bygg whisper.cpp (serverprogrammet)

Det här steget bygger **serverprogrammet** whisper.cpp — inte modellen. Modellen
(`ggml-model.bin`) är redan på plats efter Alternativ A eller B ovan. Steg 1
behöver bara göras en gång per maskin:

```powershell
cd C:\Dev
git clone https://github.com/ggml-org/whisper.cpp
cd whisper.cpp
cmake -B build -DGGML_CUDA=1 -DCMAKE_CUDA_ARCHITECTURES=120 -DWHISPER_BUILD_EXAMPLES=1
cmake --build build -j --config Release
```

- `GGML_CUDA=1` aktiverar CUDA-backend (den tidigare flaggan `WHISPER_CUBLAS`
  är utfasad).
- `CMAKE_CUDA_ARCHITECTURES=120` = Blackwell/sm_120. Samma värde för 5070 Ti
  och 5060 Ti.
- Bygget tar några minuter (kompilerar C++/CUDA).

Resultat: serverbinären hamnar i `build\bin\Release\whisper-server.exe`.

> ✅ *Verifierat 2026-06-15 på RTX 5070 Ti (hem-PC).* Bygget gick igenom med
> CUDA Toolkit 12.9 och Visual Studio Build Tools 2022 (`exit code 0`). Endast
> ofarliga kompilatorvarningar (floating-point och `size_t`-konvertering).
> Binären hamnade på
> `C:\Dev\whisper.cpp\build\bin\Release\whisper-server.exe` och CUDA-backend
> hittar kortet vid start: *NVIDIA GeForce RTX 5070 Ti, compute capability
> 12.0, 16 GB VRAM*. `system_info` rapporterar `CUDA : ARCHS = 1200` och
> `BLACKWELL_NATIVE_FP4 = 1` — rätt arkitektur byggd.

## Steg 2 — Starta servern med KB-Whisper

Kommandot pekar servern på **KB-Whisper-large** (`ggml-model.bin`) — exakt samma
modell som Diktell laddar. Justera `-m`-sökvägen om du lade modellen på en annan
plats (se Förutsättningar → Alternativ A eller B).

```powershell
C:\Dev\whisper.cpp\build\bin\Release\whisper-server.exe `
  -m C:\Dev\whisper-models\ggml-model.bin `
  --host 0.0.0.0 `
  --port 8080 `
  --inference-path /v1/audio/transcriptions `
  -l sv
```

- `-m` — sökväg till **KB-Whisper-large** (`ggml-model.bin`). Har du Diktell
  kan du även peka direkt på `C:\Dev\diktell\models\ggml-model.bin` — det är
  samma fil.
- `--host 0.0.0.0` — lyssna på alla nätverksinterface så andra maskiner på
  LAN:et kan nå servern. Default `127.0.0.1` betyder *bara den här maskinen*.
- `--port 8080` — serverns port (default).
- `--inference-path /v1/audio/transcriptions` — gör endpointen byte-identisk
  med den OpenAI/Berget-form Prata redan skickar. Default är `/inference`.
- `-l sv` — svenska som standardspråk (default är `en`). KB-Whisper är en
  svensk modell.

Servern laddar modellen i VRAM en gång vid start och håller den laddad —
ingen kallstart per anrop (till skillnad från lokal Diktell-kallstart).

> ✅ *Verifierat 2026-06-15 på RTX 5070 Ti.* Vid start laddas modellen i CUDA0
> (`whisper_model_load: type = 5 (large v3)`, 3094 MB i VRAM) och servern
> börjar lyssna på `0.0.0.0:8080`. Modellfilen är ~2,9 GiB (≈3,1 GB), samma
> KB-Whisper-large som Diktell.

> ✅ *Verifierat 2026-06-16 på rum-ett (klinik).* GPU-processen syns i
> `nvidia-smi`, endpointen svarar på `http://10.64.3.60:8080/v1/audio/transcriptions`,
> live-diktering från rum4 ger ca 1,4 s latens per anrop.

## Steg 2b — Autostarta servern vid boot (hemma)

Servern ska bete sig som Tailscale: igång efter omstart eller strömavbrott
*utan* att någon loggar in. Den körs därför som en schemalagd uppgift som
**SYSTEM, startad vid boot** (`AtStartup`) — inte knuten till din inloggning. Då
startar den i session 0 vid uppstart, behöver inget lagrat lösenord (SYSTEM har
inget), visar inget konsolfönster (session 0 har ingen interaktiv desktop), och
får auto-omstart vid krasch.

**Registrera uppgiften** (kör som administratör — UAC-ruta dyker upp):

```powershell
$TaskName  = "PrataWhisperServer"
$exe       = "C:\Dev\whisper.cpp\build\bin\Release\whisper-server.exe"
$args      = "-m ""C:\Dev\whisper-models\ggml-model.bin"" --host 0.0.0.0 --port 8080 --inference-path /v1/audio/transcriptions -l sv"
$Action    = New-ScheduledTaskAction -Execute $exe -Argument $args
$Trigger   = New-ScheduledTaskTrigger -AtStartup
$Settings  = New-ScheduledTaskSettingsSet -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries -StartWhenAvailable -ExecutionTimeLimit ([TimeSpan]::Zero) -RestartInterval (New-TimeSpan -Minutes 1) -RestartCount 3
$Principal = New-ScheduledTaskPrincipal -UserId "NT AUTHORITY\SYSTEM" -LogonType ServiceAccount -RunLevel Highest
Register-ScheduledTask -TaskName $TaskName -Action $Action -Trigger $Trigger -Settings $Settings -Principal $Principal -Force
```

- **SYSTEM / ServiceAccount / Highest** — kör vid boot utan inloggning, som en
  tjänst, utan lagrat lösenord. Eftersom session 0 saknar interaktiv desktop
  körs exe:n direkt — inget dölj-fönster-trick (VBScript) behövs.
- **`AtStartup`** — utlöses vid uppstart, oberoende av inloggning. Överlever
  omstart och strömavbrott (kommer igång vid nästa boot, precis som Tailscale).
- **`RestartCount 3` / `RestartInterval 1 min`** — kraschar servern startas den
  om, upp till tre gånger med en minuts mellanrum.
- **`ExecutionTimeLimit 0`** — ingen tidsgräns; servern får köra hur länge som
  helst.

> **GeForce + session 0:** den enda tekniska osäkerheten var om CUDA fungerar för
> SYSTEM i session 0 (inget interaktivt skrivbord). Det är verifierat att det
> gör det på den här maskinen (se noten nedan) — ren GPU-beräkning behöver ingen
> desktop.

**Starta nu** utan att vänta på en omstart (kräver administratör eftersom
uppgiften körs som SYSTEM — vid en riktig boot startar Schemaläggaren den själv
utan UAC):

```powershell
Start-ScheduledTask -TaskName PrataWhisperServer
```

**Datorn får inte själv somna.** Skärmens viloläge är ofarligt (servern
påverkas inte), men om hela maskinen går i vila/viloläge (S3/hibernate) stannar
GPU:n och nätet — då är servern onåbar. Sätt systemets timeouts till 0 på
nätström (skärmen får gärna släckas):

```powershell
powercfg /change standby-timeout-ac 0     # datorn somnar aldrig på nätström
powercfg /change hibernate-timeout-ac 0
```

**Hantering** (start/stopp/borttagning kräver administratör eftersom uppgiften
ägs av SYSTEM):

```powershell
Get-ScheduledTask -TaskName PrataWhisperServer                          # status
Get-Process whisper-server | Select Id, StartTime                       # körs den?
Stop-ScheduledTask  -TaskName PrataWhisperServer                        # stoppa servern
Unregister-ScheduledTask -TaskName PrataWhisperServer -Confirm:$false   # ta bort autostart
```

> ✅ *Verifierat 2026-06-15 på hem-PC:n.* Uppgiften kör som
> `NT AUTHORITY\SYSTEM`, port 8080 lyssnar på `0.0.0.0` och ett
> transkriberings-anrop mot den SYSTEM-startade instansen gav korrekt JSON —
> dvs **CUDA/GPU fungerar i session 0**. Ströminställningarna är redan rätt
> (`standby-timeout-ac = 0`, `hibernate-timeout-ac = 0`), så datorn somnar aldrig
> av sig själv på nätström. Nettoresultat: servern startar vid boot utan
> inloggning och överlever omstart/strömavbrott — som Tailscale.

> ✅ *Verifierat 2026-06-16 på rum-ett (klinik).* Uppgiften **`PrataWhisperServer`**
> startar servern vid boot utan inloggning. Se avsnittet om brandväggen under
> Steg 4 — SYSTEM-autostart triggar **aldrig** den interaktiva brandväggsdialogen,
> så en explicit inbound-regel måste läggas manuellt.

> **Jobbet:** samma uppgift gäller där, men följ klinikens IT-policy för
> tjänster/autostart. Skillnaden är brandväggsregeln (LAN, inte Tailscale).

## Steg 3 — Verifiera servern

**Lokalt på servern.** Kontrollera att porten lyssnar och gör ett litet
test-anrop med whisper.cpp:s medföljande exempelljud (`samples\jfk.wav`):

```powershell
# lyssnar porten?
(Test-NetConnection 127.0.0.1 -Port 8080).TcpTestSucceeded   # -> True

# transkriberings-anrop (jfk.wav är engelska, så sätt language=en just här)
curl.exe -X POST "http://127.0.0.1:8080/v1/audio/transcriptions" `
  -F "file=@C:\Dev\whisper.cpp\samples\jfk.wav" `
  -F "response_format=json" `
  -F "language=en"
```

Svaret ska vara OpenAI-kompatibel JSON, t.ex.:

```json
{"text":" And so, my fellow Americans, ask not what your country can do for you, ask what you can do for your country."}
```

> ✅ *Verifierat lokalt 2026-06-15 på RTX 5070 Ti.* Porten svarade och
> endpointen returnerade korrekt JSON. Inferens ~1,3 s för ett 11-sekunders
> klipp på GPU:n.

**Hemma — externt över Tailscale.** Hemmaservern nås alltid externt (aldrig
från en maskin på samma fysiska hemnät), så det meningsfulla testet görs från
en off-nät enhet, t.ex. en MacBook tjudrad till mobilen. Byt `127.0.0.1` mot
serverns **Tailscale-IP** (`100.87.6.56`). curl finns inbyggt i macOS:

```bash
curl -X POST "http://100.87.6.56:8080/v1/audio/transcriptions" \
  -F "file=@jfk.wav" \
  -F "response_format=json" \
  -F "language=en"
```

Detta bevisar hela produktionsvägen hemma: off-nät klient → Tailscale → GPU.
Notera: testet verifierar *serverns nåbarhet*. Själva Prata-klienten (Steg 5)
är Windows-only och körs inte på Mac:en — Mac:en är ett uppkopplingstest, inte
en dikteringsklient.

> ⏳ *Verifieras när servern körs och MacBook:en kopplas via Tailscale.*

**Jobbet — internt på klinikens LAN.** På jobbet är det tvärtom: dikteringen
sker från andra arbetsstationer på *samma* nät som GPU-maskinen, och patientljud
får aldrig lämna det nätet. Testa från klientmaskinen (t.ex. rum4), **inte bara
lokalt på servern** — se avsnittet "Servern fungerar lokalt men inte från
klienten" under Felsökning.

PowerShell **på klientmaskinen** (t.ex. rum4):

```powershell
Test-NetConnection 10.64.3.60 -Port 8080
# TcpTestSucceeded : True  ← krävs innan Prata kan diktera
```

> ✅ *Verifierat 2026-06-16.* Från rum4 (`10.64.3.59`) mot rum-ett (`10.64.3.60`):
> `TcpTestSucceeded : True` efter brandväggsregeln lagts på servern. Live-diktering
> mot Rum1 GPU-server fungerar.

## Steg 4 — Brandväggsregel (inbound)

Windows Defender släpper inte in trafik till serverporten förrän en inbound-regel
lagts. **Det här är det vanligaste felet vid ny driftsättning** — servern verkar
fungera på GPU-maskinen men når inte från klienter (se Felsökning).

> **SYSTEM-autostart och brandväggen.** När `whisper-server` startas via
> schemaläggningsuppgiften (`PrataWhisperServer`, SYSTEM vid boot) triggas **aldrig**
> den interaktiva brandväggsdialogen som dyker upp vid manuell start. Inkommande
> TCP 8080 är därför blockerat som standard tills du lägger en explicit regel —
> även om servern lyssnar på `0.0.0.0:8080`.

Vilken regel beror på *platsen* — och de två platserna har medvetet olika
åtkomstmodell:

| Plats | Åtkomst | Regel |
|---|---|---|
| **Hemma** | Alltid externt (Tailscale). Ingen klient sitter på samma fysiska hemnät. | Endast Tailscale-regel. |
| **Jobbet** | Alltid internt på klinikens LAN. Patientljud lämnar aldrig nätet. | Endast LAN-regel (LocalSubnet). |

### Hemma — Tailscale-regel

Hemmaservern når du från dina egna enheter (Landet-PC, MacBook) över Tailscale.
Regeln scopas till tailnet-intervallet (`100.64.0.0/10`) så bara dina egna
Tailscale-enheter kan nå porten — inte internet i stort, eftersom serverns
`100.x`-adress bara är nåbar genom den krypterade tunneln. Kör som
administratör (UAC-ruta dyker upp om sessionen inte är förhöjd):

```powershell
New-NetFirewallRule `
  -DisplayName "Prata whisper-server (Tailscale)" `
  -Direction Inbound -Action Allow -Protocol TCP -LocalPort 8080 `
  -RemoteAddress 100.64.0.0/10 `
  -Profile Any
```

- `-RemoteAddress 100.64.0.0/10` — Tailscales CGNAT-intervall. Säkerhetsspärren
  är att adressen bara är nåbar via tunneln.
- `-Profile Any` — Tailscale-adapterns nätverksprofil varierar; scopningen på
  remote-adress är den faktiska spärren, inte profilen.
- Klienter pekar på hemmaserverns **Tailscale-IP** (`100.87.6.56`), inte LAN-IP.
  Ytterligare åtstramning kan göras i Tailscales egna ACL:er (vilka enheter som
  får nå port 8080).
- Någon LAN-regel (`LocalSubnet`) behövs *inte* hemma — ingen klient sitter på
  hemnätet, så port 8080 hålls stängd mot LAN för minsta exponering.

> ✅ *Verifierat 2026-06-15 på hem-PC:n.* Regeln är aktiv: `Enabled = True`,
> `Inbound`, `Allow`, `TCP/8080`, `Profile = Any`,
> `RemoteAddress = 100.64.0.0/10`. (En tidigare LocalSubnet-regel togs bort —
> hemma används bara den externa Tailscale-vägen.) ⏳ Test-anrop från en
> off-nät enhet (MacBook via mobil) över Tailscale återstår.

### Jobbet — LAN-regel

På jobbet sker dikteringen från andra arbetsstationer på klinikens *eget* nät,
och Tailscale är uteslutet (patientljud får inte lämna nätet). Regeln begränsas
till lokala nätet:

```powershell
New-NetFirewallRule `
  -DisplayName "Prata whisper-server (LAN)" `
  -Direction Inbound -Action Allow -Protocol TCP -LocalPort 8080 `
  -RemoteAddress LocalSubnet `
  -Profile Domain
```

- `-RemoteAddress LocalSubnet` håller åtkomsten inom klinikens nät — inga
  anslutningar utifrån.
- `-Profile Domain` är den sannolika profilen på ett klinik-/regionnät.
  Brandvägg och portöppning kan dock styras centralt av klinikens IT — följ
  deras policy i stället för att öppna porten på egen hand.

**Diagnosregel (vid felsökning).** Om du behöver snabbt bekräfta att brandväggen
är rotorsaken kan du temporärt lägga en bredare regel — kör som administratör
**på GPU-servern**:

```powershell
New-NetFirewallRule -DisplayName "Prata Whisper Server 8080" `
  -Direction Inbound -Protocol TCP -LocalPort 8080 -Action Allow -Profile Any
```

Verifiera därefter från klientmaskinen (`Test-NetConnection 10.64.3.60 -Port 8080`).
Strama sedan åt regeln (servern saknar auth):

```powershell
Set-NetFirewallRule -DisplayName "Prata Whisper Server 8080" `
  -RemoteAddress 10.64.3.0/24
# eller explicit lista: -RemoteAddress 10.64.3.59,10.64.3.61
```

> ⚠️ **Viktigt — nätmasken begränsar vem som når servern.** På GPU-maskinen är
> nätmasken `255.255.255.192`. Windows `LocalSubnet` i brandväggsregeln ovan
> motsvarar det adressområde som hör ihop med den nätmasken på servern — inte
> hela klinikens nät. Ligger dikterings-arbetsstationerna utanför det
> området (troligt om de har annan IP/nätmask) når de inte servern. Då måste
> `-RemoteAddress` vidgas till klienternas adressområde i stället för
> `LocalSubnet` — men håll det internt, aldrig internet.

> ✅ *Verifierat 2026-06-16 på rum-ett.* Rotorsak till att rum4 inte nådde servern
> var **saknad inbound-regel**, inte LAN-segmentering (rum4 `10.64.3.59` och
> rum-ett `10.64.3.60` ligger i samma subnät). Efter regeln:
> `Test-NetConnection` från rum4 → `TcpTestSucceeded : True`, live-diktering
> fungerar. Jobbservern får *aldrig* en Tailscale-regel.

## Steg 5 — Peka Prata mot servern

Prata har en **backend-väljare** med tre alternativ i tray-menyn:

| Plats (infra) | Tray-ID (sparas i `backend.txt`) | Visningsnamn i menyn |
|---|---|---|
| Hem-GPU (Tailscale) | `Hemma` | Rngv GPU-server |
| Jobb-GPU (LAN) | `Jobb` | Rum1 GPU-server |
| Moln | `Berget` | Berget Ai |

Rngv och Rum1 är lokala whisper.cpp-GPU-servrar (ingen auth); Berget Ai är
moln-fallbacken (Bearer-autentiserad). ID:t är stabilt och sparas i
`backend.txt`; visningsnamnen kan ändras i kod utan att bryta befintliga val.

### Endpoint-konstanter

Endpointerna är hårdkodade konstanter i `internal/transcribe/client.go`
(Prata följer "ändra konstant + kompilera om", ingen config-fil):

```go
const (
	HomeURL   = "http://100.87.6.56:8080/v1/audio/transcriptions" // hem-GPU via Tailscale
	WorkURL   = "http://10.64.3.60:8080/v1/audio/transcriptions"  // jobb-GPU, fast LAN-IP
	BergetURL = "https://api.berget.ai/v1/audio/transcriptions"
)
```

- **HomeURL** pekar på hem-serverns **Tailscale-IP**, så den fungerar oavsett
  vilket nät klienten sitter på (stuga, mobil-hotspot, hemnät).
- **WorkURL** pekar på jobb-serverns **fasta LAN-IP** (`10.64.3.60`). Den nås
  bara inifrån klinikens nät, så att välja Rum1 GPU-server utanför kliniken ger
  felton — aldrig tyst fallback. Ändras serverns adress: uppdatera konstanten
  och bygg om.

### Hur klienten hittar servern (adressering)

`WorkURL` är en kompilerad konstant — adressen bakas in i `prata.exe`. Servern
lyssnar på `0.0.0.0:8080` och svarar oavsett om den nås via IP eller namn.

**Nätverksinställningar på kliniken.** Vid varje ny datorinstallation knappas
nätverket in manuellt på nätverkskortet (IPv4, statisk konfiguration — inte
DHCP). **DNS är samma på alla maskiner**; **IP och nätmask varierar per dator**
och sätts utifrån vilken plats i nätet maskinen får. GPU-servern är ingen
undantag — den får sin egen IP och subnät precis som övriga arbetsstationer.

| Värde | GPU-servern (rum-ett) | Klient (rum4) | Övriga datorer |
|---|---|---|---|
| IP | `10.64.3.60` | `10.64.3.59` | annan per maskin |
| Nätmask | `255.255.255.192` | samma subnät | kan variera per maskin |
| DNS 1 | `192.44.242.131` | samma | samma |
| DNS 2 | `192.44.243.131` | samma | samma |

`WorkURL` pekar på **GPU-maskinens** IP — inte klientens. På den här
installationen:

```go
WorkURL = "http://10.64.3.60:8080/v1/audio/transcriptions"
```

Byter GPU-servern IP (ny maskin, annan nätmask): uppdatera `WorkURL` till den nya
adressen och bygg om Prata. DNS-adresserna behöver normalt inte ändras.

Brandväggsregeln (`LocalSubnet`) följer serverns nätmask — se varningen under
Steg 4 → Jobbet om dikterings-arbetsstationerna har annan IP/nätmask än servern.

Värdnamn fungerar också (delad DNS), t.ex.
`http://RBS-PC:8080/v1/audio/transcriptions`, om GPU-maskinen är registrerad i
DNS. Med manuellt satta IP:n per maskin är det enklast att använda IP direkt i
`WorkURL`.

Två saker att hålla reda på oavsett val:

- `WorkURL` är en konstant, så en *ändrad* adress kräver ombyggnad av Prata och
  omdistribution till arbetsstationerna.
- Klient och server måste ligga på samma LAN/subnät som brandväggsregeln
  (`LocalSubnet`) tillåter — sitter klienterna på ett annat subnät/VLAN måste
  regeln vidgas, vilket klinikens IT i så fall styr.

### Villkorlig auth

Bara Berget Ai skickar `Authorization: Bearer <nyckel>`. De lokala GPU-servrarna
får aldrig nyckeln. API-nyckeln laddas numera "best-effort": saknas den startar
Prata ändå (lokala backends behöver ingen), men Berget Ai rapporterar fel om det
väljs utan nyckel.

### Välja backend

Högerklicka tray-ikonen → välj **Rngv GPU-server / Rum1 GPU-server / Berget Ai**
(radioknappar, aktivt val är prickat). Aktiv backend visas alltid:

- i tray-tooltipen (`Prata — Rngv GPU-server`), och
- som en balong när du byter (`Aktiv transkribering: Rngv GPU-server`).

Valet sparas som stabilt ID (`Hemma`, `Jobb` eller `Berget`) i
`%LOCALAPPDATA%\Prata\backend.txt` och överlever omstart. **Standard vid första
start** (saknad eller ogiltig `backend.txt`) är **Rum1 GPU-server** (`Jobb`) —
den interna GPU-servern på klinik-LAN, som inte kräver API-nyckel. Välj
**Rngv GPU-server** hemma (Tailscale) eller **Berget Ai** i menyn vid behov.

**Ingen tyst failover:** är vald server nere får du felton vid diktering, inte
ett tyst byte till en annan backend. Byte sker bara när *du* väljer i menyn.

### Testa

**Klinik (rum4 → rum-ett):**

1. Bekräfta att GPU-servern kör på rum-ett (`Get-Process whisper-server` eller
   att schemaläggningsuppgiften `PrataWhisperServer` är igång).
2. Från rum4: `Test-NetConnection 10.64.3.60 -Port 8080` → `TcpTestSucceeded : True`.
3. Kör Prata på rum4. Högerklicka tray-ikonen → **Rum1 GPU-server**.
4. Håll **F1**, diktera, släpp — texten transkriberas mot GPU-servern (~1,4 s).

**Hemma (Tailscale):**

1. Starta servern på hem-PC:n (Steg 2-kommandot eller autostart).
2. Kör Prata. Högerklicka tray-ikonen → **Rngv GPU-server**.
3. Håll **F1**, diktera, släpp.

Snabbtest av nåbarhet utan Prata (t.ex. från MacBook över Tailscale) — se
Steg 3.

> ✅ *Implementerat och verifierat 2026-06-15:* bygger rent (`go build`,
> `go vet`), enhetstester för backend-routing och villkorlig auth passerar.
> ✅ *Live-diktering verifierad 2026-06-16:* rum4 → rum-ett (Rum1 GPU-server,
> LAN, ~1,4 s latens). ⏳ Live-diktering mot Rngv GPU-server (Tailscale) återstår
> som separat test.

## Installationsprompt — jobb-PC (klistra in i Cursor/Claude)

Tanken med jobb-driftsättningen: checka ut Prata-repot på jobb-maskinen (det här
dokumentet följer med), öppna Cursor/Claude i repo-mappen och klistra in prompten
nedan. Agenten kör hela serveruppsättningen för **jobb-scenariot** (LAN-only) och
stannar bara där du behöver godkänna (UAC) eller fatta ett beslut. Allt agenten
behöver står i det här dokumentet.

```text
Du sätter upp Prata-GPU-servern på en JOBB-PC (klinik). Hela guiden finns i
PRATA-GPU-SERVER.md i den här mappen — läs den först och följ den. Kör från
Prata-repots rot (där dokumentet ligger). Det här är JOBB-scenariot, inte hemma:

- Endast LAN. Tailscale är UTESLUTET. Patientljud får ALDRIG lämna klinikens nät.
- Brandväggen scopas till LocalSubnet/Domain — lägg ALDRIG en Tailscale-regel.
- Lämna HomeURL orörd. Rör inga hemma-specifika delar.

Arbeta autonomt. Stanna bara för (a) UAC-godkännanden och (b) beslut som kräver
mig. Slå ihop ALLA förhöjda steg i ETT enda elevererat anrop så jag bara behöver
godkänna en (1) UAC-ruta. Verifiera varje steg innan du går vidare; rapportera
kort vad som gjordes.

Steg:

1. FÖRUTSÄTTNINGAR. Kör `nvidia-smi`, fastställ GPU-modell och compute
   capability och sätt `CMAKE_CUDA_ARCHITECTURES` därefter (RTX 50-serien =
   Blackwell sm_120 -> 120; annat kort -> slå upp rätt arch-nummer). Kontrollera
   att CUDA Toolkit (>=12.8 för Blackwell), CMake 3.20+ och Visual Studio Build
   Tools 2022 finns. Bygger maskinen redan Diktell finns kedjan på plats; annars
   följ avsnittet Förutsättningar. Ladda ner modellen (`ggml-model.bin`, samma
   som Diktell) om den saknas.

2. BYGG. Följ Steg 1 (whisper.cpp med CUDA) med rätt arch-värde. Verifiera att
   `whisper-server.exe` byggts och att CUDA-backend hittar kortet.

3. NÄTVERKSKORT. Nätverket knappas in manuellt på nätverkskortet (IPv4, statisk).
   DNS är alltid `192.44.242.131` och `192.44.243.131` (samma på alla maskiner).
   IP och nätmask varierar per dator — på GPU-servern (den här maskinen) är det
   IP `10.64.3.60`, nätmask `255.255.255.192`. Bekräfta med `ipconfig` att
   inställningarna stämmer; notera IP:n — den blir `WorkURL`.

4. ETT FÖRHÖJT ANROP (en UAC) som gör allt nedan i följd:
   a. Brandvägg: lägg LAN-regeln från "Steg 4 -> Jobbet". Om osäker, använd
      diagnosregeln (-Profile Any) först och verifiera från klientmaskinen med
      Test-NetConnection; strama sedan åt med -RemoteAddress. OBS: SYSTEM-autostart
      triggar ALDRIG brandväggsdialogen — regeln måste läggas explicit. Om
      LocalSubnet inte räcker (klient utanför serverns subnät), stanna och fråga
      mig om rätt adressområde. Blockeras regeln eller styrs av GPO -> stanna
      och be mig involvera klinikens IT.
   b. Strömläge: `powercfg /change standby-timeout-ac 0` och
      `powercfg /change hibernate-timeout-ac 0` så maskinen inte somnar (skärmen
      får släckas). Styrs detta av GPO -> notera det.
   c. Autostart: registrera SYSTEM/boot-uppgiften EXAKT som i Steg 2b
      (AtStartup, ServiceAccount/Highest, restart 3x1 min, ExecutionTimeLimit 0).
      Styr klinikens IT tjänster/autostart centralt -> stäm av med dem.
   d. Starta uppgiften (Start-ScheduledTask).

5. VERIFIERA. Vänta in modell-laddningen, bekräfta att processen kör som
   NT AUTHORITY\SYSTEM, att port 8080 lyssnar på 0.0.0.0. Testa från en
   ANDRA arbetsstation på LAN:et (inte bara lokalt på servern). Test-NetConnection
   mot serverns LAN-IP ska ge TcpTestSucceeded : True. Gör ett transkriberings-anrop
   (Steg 3).

6. PEKA PRATA MOT SERVERN. `WorkURL` är redan satt i
   `internal/transcribe/client.go` till `http://10.64.3.60:8080/v1/audio/transcriptions`
   (jobb-serverns fasta IP). Bekräfta att IP:n stämmer; ändra bara om servern
   fått en annan adress. Bygg om Prata (`go build …` eller `install.ps1 -Local`
   för legacy-install). Notera att den ombyggda `prata.exe` måste distribueras till de arbetsstationer som ska
   diktera mot servern. Verifiera att backend Rum1 GPU-server går att välja och dikterar.

7. SLUTRAPPORT. Uppdatera jobb-statusraderna i PRATA-GPU-SERVER.md och ge mig:
   GPU/arch, modellsökväg, brandväggsregel, autostart-status, verifieringsresultat
   och vilken WorkURL som sattes.
```

## Felsökning

### Servern fungerar lokalt men inte från klienten

**Symptom:** GPU-servern svarar när du öppnar `http://10.64.3.60:8080` *på
servermaskinen*, men klienten (t.ex. rum4) får timeout eller Prata spelar
felton vid diktering.

**Varför det ser förvirrande ut:** lokal åtkomst till maskinens egen IP går via
loopback och **passerar aldrig Windows-brandväggen**. En anslutning från en
annan maskin gör det. Att servern "fungerar lokalt" bevisar därför *inte* att
klienter kan nå den — bara att processen lyssnar.

**Felsökningsordning:**

| Steg | Hypotes | Kommando / åtgärd | Typiskt utfall |
|---|---|---|---|
| 1 | Servern nere | `Get-Process whisper-server` på GPU-maskinen | Process saknas → starta uppgiften |
| 2 | Fel port | `Test-NetConnection 10.64.3.60 -Port 8080` från klient | Servern lyssnar på **8080**, inte 8000 |
| 3 | Bunden till localhost | Testa `10.64.3.60` lokalt *på servern* | Fungerar lokalt men inte från klient → brandvägg, inte bindning |
| 4 | LAN-segmentering | `Test-NetConnection` från klient, titta på `SourceAddress` | Samma subnät (t.ex. `10.64.3.59 → 10.64.3.60`) → segmentering utesluten |
| 5 | **Brandvägg** | Lägg inbound-regel (Steg 4 → Jobbet) | **Vanligaste rotorsaken** vid SYSTEM-autostart |

**Verifiering efter brandväggsfix** (PowerShell på klientmaskinen):

```powershell
Test-NetConnection 10.64.3.60 -Port 8080
# TcpTestSucceeded : True
```

> ✅ *Verifierat 2026-06-16:* rum4 kunde inte nå rum-ett trots att servern svarade
> lokalt. Rotorsak: saknad inbound-regel. Efter regeln: live-diktering fungerar.

### Verifiera att KB-Whisper-Large körs

KB-Whisper-Large är en finjustering av OpenAI:s whisper-large — identisk
arkitektur och vokabulär. Modellens interna metadata avslöjar **inte** om det är
KB-versionen. Verifiera istället på två sätt:

1. **Ursprung (identitet).** Kontrollera vilken `.bin` servern startades med:

   ```powershell
   Get-CimInstance Win32_Process -Filter "Name='whisper-server.exe'" |
     Select-Object -ExpandProperty CommandLine
   ```

   Titta på `-m`/`--model`-argumentet. En `ggml-model.bin` från
   `KBLab/kb-whisper-large` är rätt; ett generiskt `ggml-large-v3.bin` från
   whisper.cpp:s standardnedladdning är fel modell.

2. **Beteende (klinisk bekräftelse).** Diktera samma svenska mening med medicinska
   termer mot **Rum1 GPU-server** respektive **Berget Ai** (bekräftad
   KB-Whisper-Large) och jämför. Identiskt felmönster ⇒ samma modell, och
   `dictionary-corrections.txt` (baslinjen är inbäddad i binären; override-filen
   per användare ligger i `%LOCALAPPDATA%\Prata\`) är rätt kalibrerad.

### Prata-klientinstallation — kända problem

- **AV/EDR-blockering:** osignerad `prata.exe` kan blockeras av Webroot/SmartScreen
  på nya maskiner (loaderfel utan krasch). Allowlista `%ProgramFiles%\Prata`
  (maskinbred install) eller `%LOCALAPPDATA%\Prata` (legacy per-användare-install),
  eller signera binären.
- **Maskinbred install (rekommenderat på klinik):** kopiera `prata.exe` från USB
  och kör `prata.exe --install` (UAC). Binären hamnar i
  `%ProgramFiles%\Prata\`, autostart registreras maskinbrett för alla användare.
  Per-användardata (`apikey.dat`, `backend.txt`, dictionary-override) skapas
  automatiskt under `%LOCALAPPDATA%\Prata\` vid behov. Hårdvaru-smoke-test
  deferrat — se PRATA-DESIGN-LOG.md (2026-06-17).
- **Legacy / offline:** `install.ps1 -Local` bygger från källkod och kräver Go +
  C-verktygskedja. Installerar till `%LOCALAPPDATA%\Prata` med per-användare-
  autostart. För maskiner utan verktygskedja: kopiera förbyggd `prata.exe` och
  kör `--install`, eller kopiera manuellt till legacy-sökvägen.
- **Autostart:** schemaläggningsuppgiften **`Prata`** startar klienten vid
  inloggning (skilj från serveruppgiften **`PrataWhisperServer`** på GPU-maskinen).
  Maskinbred install: en uppgift för alla användare (RunLevel Limited). Legacy
  `install.ps1`: en uppgift per användare.

### Öppna säkerhetspunkter

- **Strama åt brandväggsregeln** på GPU-servern: byt från `-Profile Any` till
  `-RemoteAddress` med kända klient-IP:n (se Steg 4 → Jobbet).
- **Rotera Berget-nyckeln** om den exponerats i klartext under felsökning; kör
  `prata.exe --set-key` (eller legacy `prata-setkey`) igen på berörda maskiner.

## Status

| Steg | Status |
|---|---|
| Arkitektur & principer | Klar |
| Förutsättningar | Klar |
| Steg 1 — Bygg whisper.cpp | ✅ Verifierat på RTX 5070 Ti (hem-PC) |
| Steg 2 — Starta servern | ✅ Verifierat — modell laddad på GPU, lyssnar |
| Steg 2b — Autostart (hemma) | ✅ Körs som SYSTEM vid boot — överlever omstart/strömavbrott, GPU verifierad i session 0 |
| Steg 3 — Verifiera | ✅ Lokalt + LAN från rum4→rum-ett (2026-06-16); ⏳ Tailscale-test (hemma) kvar |
| Steg 4 — Brandvägg (hemma/Tailscale) | ✅ Regel aktiv på hem-PC:n (ringvagen) |
| Steg 4 — Brandvägg (jobbet/LAN) | ✅ Verifierad på rum-ett (2026-06-16); rotorsak vid fel: saknad inbound-regel |
| Steg 5 — Prata-klient | ✅ Live-diktering rum4→rum-ett (~1,4 s); Rum1 GPU-server produktionsväg; maskinbred `--install` implementerad (smoke-test deferrat) |
