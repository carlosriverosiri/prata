# Prata — GPU-server för LAN-transkribering

> **Status:** utkast under uppbyggnad. Fylls på steg för steg allt eftersom
> varje steg verifieras på riktig hårdvara.
> **Senast uppdaterad:** 2026-06-15

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

## Förutsättningar

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

> **Jobbet:** samma uppgift kan sättas upp där, men följ klinikens IT-policy för
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
får aldrig lämna det nätet. Byt då `127.0.0.1` mot serverns LAN-IP (`ipconfig`
på servern). Kräver LAN-brandväggsregeln (Steg 4 → Jobbet).

> ⏳ *Verifieras på plats på jobbet med en andra arbetsstation.*

## Steg 4 — Brandväggsregel (inbound)

Windows Defender släpper inte in trafik till serverporten förrän en inbound-regel
lagts. Vilken regel beror på *platsen* — och de två platserna har medvetet olika
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

> ⚠️ **Viktigt — nätmasken begränsar vem som når servern.** På GPU-maskinen är
> nätmasken `255.255.255.192`. Windows `LocalSubnet` i brandväggsregeln ovan
> motsvarar det adressområde som hör ihop med den nätmasken på servern — inte
> hela klinikens nät. Ligger dikterings-arbetsstationerna utanför det
> området (troligt om de har annan IP/nätmask) når de inte servern. Då måste
> `-RemoteAddress` vidgas till klienternas adressområde i stället för
> `LocalSubnet` — men håll det internt, aldrig internet.

> ⏳ *Läggs och verifieras på plats på jobbet.* Jobbservern får *aldrig* en
> Tailscale-regel.

## Steg 5 — Peka Prata mot servern

Prata har nu en **backend-väljare** med tre alternativ: **Hemma**, **Jobb** och
**Berget**. Hemma och Jobb är lokala whisper.cpp-GPU-servrar (ingen auth);
Berget är moln-fallbacken (Bearer-autentiserad).

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
  bara inifrån klinikens nät, så att välja "Jobb" utanför kliniken ger felton —
  aldrig tyst fallback. Ändras serverns adress: uppdatera konstanten och bygg om.

### Hur klienten hittar servern (adressering)

`WorkURL` är en kompilerad konstant — adressen bakas in i `prata.exe`. Servern
lyssnar på `0.0.0.0:8080` och svarar oavsett om den nås via IP eller namn.

**Nätverksinställningar på kliniken.** Vid varje ny datorinstallation knappas
nätverket in manuellt på nätverkskortet (IPv4, statisk konfiguration — inte
DHCP). **DNS är samma på alla maskiner**; **IP och nätmask varierar per dator**
och sätts utifrån vilken plats i nätet maskinen får. GPU-servern är ingen
undantag — den får sin egen IP och subnät precis som övriga arbetsstationer.

| Värde | GPU-servern (den här maskinen) | Övriga datorer |
|---|---|---|
| IP | `10.64.3.60` | annan per maskin |
| Nätmask | `255.255.255.192` | kan variera per maskin |
| DNS 1 | `192.44.242.131` | samma |
| DNS 2 | `192.44.243.131` | samma |

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

Bara Berget skickar `Authorization: Bearer <nyckel>`. De lokala GPU-servrarna
får aldrig nyckeln. API-nyckeln laddas numera "best-effort": saknas den startar
Prata ändå (lokala backends behöver ingen), men Berget rapporterar fel om det
väljs utan nyckel.

### Välja backend

Högerklicka tray-ikonen → välj **Hemma / Jobb / Berget** (radioknappar, aktivt
val är prickat). Aktiv backend visas alltid:

- i tray-tooltipen (`Prata — Hemma`), och
- som en balong när du byter (`Aktiv transkribering: Hemma`).

Valet sparas i `%LOCALAPPDATA%\Prata\backend.txt` och överlever omstart.
Standard vid första start är **Berget** (fungerar överallt med nyckel).

**Ingen tyst failover:** är vald server nere får du felton vid diktering, inte
ett tyst byte till en annan backend. Byte sker bara när *du* väljer i menyn.

### Testa

1. Starta servern på GPU-maskinen (Steg 2-kommandot).
2. Kör Prata (`prata.exe`, eller `go run ./cmd/prata` för loggar i terminalen).
3. Högerklicka tray-ikonen → **Hemma**. Tooltipen blir `Prata — Hemma`.
4. Håll **F1**, diktera, släpp — texten transkriberas mot GPU-servern.

Snabbtest av nåbarhet utan Prata (t.ex. från MacBook över Tailscale) — se
Steg 3.

> ✅ *Implementerat och verifierat 2026-06-15:* bygger rent (`go build`,
> `go vet`), enhetstester för backend-routing och villkorlig auth passerar
> (Berget skickar Bearer + fält; lokal backend skickar ingen auth även med
> nyckel satt; tom URL och Berget-utan-nyckel felar). ⏳ Live-diktering mot
> Hemma-servern testas av användaren med servern igång.

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
   a. Brandvägg: lägg LAN-regeln från "Steg 4 -> Jobbet" (LocalSubnet, Profile
      Domain). OBS: `LocalSubnet` följer serverns nätmask — om
      dikterings-arbetsstationerna har annan IP/nätmask når de inte servern.
      Stanna och fråga mig om rätt adressområde (vidga `-RemoteAddress`, håll
      det internt). Blockeras regeln eller styrs av GPO -> stanna och be mig
      involvera klinikens IT.
   b. Strömläge: `powercfg /change standby-timeout-ac 0` och
      `powercfg /change hibernate-timeout-ac 0` så maskinen inte somnar (skärmen
      får släckas). Styrs detta av GPO -> notera det.
   c. Autostart: registrera SYSTEM/boot-uppgiften EXAKT som i Steg 2b
      (AtStartup, ServiceAccount/Highest, restart 3x1 min, ExecutionTimeLimit 0).
      Styr klinikens IT tjänster/autostart centralt -> stäm av med dem.
   d. Starta uppgiften (Start-ScheduledTask).

5. VERIFIERA. Vänta in modell-laddningen, bekräfta att processen kör som
   NT AUTHORITY\SYSTEM, att port 8080 lyssnar på 0.0.0.0 och gör ett
   transkriberings-anrop (Steg 3) — det bevisar även att GPU fungerar i
   session 0. Testa om möjligt även från en andra arbetsstation på LAN:et mot
   serverns LAN-IP.

6. PEKA PRATA MOT SERVERN. `WorkURL` är redan satt i
   `internal/transcribe/client.go` till `http://10.64.3.60:8080/v1/audio/transcriptions`
   (jobb-serverns fasta IP). Bekräfta att IP:n stämmer; ändra bara om servern
   fått en annan adress. Bygg om Prata (`install.ps1 -Local`). Notera att den
   ombyggda `prata.exe` måste distribueras till de arbetsstationer som ska
   diktera mot servern. Verifiera att backend "Jobb" går att välja och dikterar.

7. SLUTRAPPORT. Uppdatera jobb-statusraderna i PRATA-GPU-SERVER.md och ge mig:
   GPU/arch, modellsökväg, brandväggsregel, autostart-status, verifieringsresultat
   och vilken WorkURL som sattes.
```

## Felsökning

> ⏳ *Växer allt eftersom vi stöter på saker.*

## Status

| Steg | Status |
|---|---|
| Arkitektur & principer | Klar |
| Förutsättningar | Klar |
| Steg 1 — Bygg whisper.cpp | ✅ Verifierat på RTX 5070 Ti (hem-PC) |
| Steg 2 — Starta servern | ✅ Verifierat — modell laddad på GPU, lyssnar |
| Steg 2b — Autostart (hemma) | ✅ Körs som SYSTEM vid boot — överlever omstart/strömavbrott, GPU verifierad i session 0 |
| Steg 3 — Verifiera | ✅ Lokalt verifierat; ⏳ Tailscale-test (hemma) + LAN-test (jobbet) kvar |
| Steg 4 — Brandvägg (hemma/Tailscale) | ✅ Regel aktiv på hem-PC:n; LAN-regel borttagen |
| Steg 4 — Brandvägg (jobbet/LAN) | ⏳ Läggs på plats på jobbet (obs LocalSubnet vs klienternas nätmask) |
| Steg 5 — Prata-klient | ✅ Backend-väljare byggd och testad; WorkURL satt till `10.64.3.60` (jobb); ⏳ live-diktering kvar |
