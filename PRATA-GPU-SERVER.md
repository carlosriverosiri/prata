# Prata — GPU server for LAN transcription

> **Status:** draft under construction. Filled in step by step as
> each step is verified on real hardware.
> **Last updated:** 2026-06-16

## Purpose

This guide describes how to set up a local KB-Whisper server on a machine
with an RTX 50 card (Blackwell), so that Prata can dictate against it over
the network instead of against Berget AI. Written so it can be reproduced on a
new machine in the future.

The server runs the same model as Diktell (KB-Whisper-large GGML) but exposes it
as an OpenAI-compatible HTTP endpoint, which is exactly the form Prata already
speaks.

## Architecture and principles

A brief note on why the setup looks the way it does — it governs all the choices below.

**One GPU server per location, local to its own network.** Each location (home, clinic)
has its own server on its own network. The two worlds — home and clinic — are
never bridged together. At the clinic, client and server always sit on the same physical network.

**At home, remote access is allowed within your own tailnet.** The home server may be reached
from another of your *own* machines (e.g. the country-house PC) over Tailscale. This
is not a bridge between home and clinic — it is your private mesh between your
own devices, and home use is for private dictation, not clinical
patient audio. The clinic server is *never* exposed over Tailscale (see Step 4).

**Patient audio never leaves the network.** At the clinic, all audio data stays within
the clinic's/region's network — client → server is internal traffic. Sending clinical
audio data out to a private machine is out of the question (GDPR/information security),
the same logic that led to Berget being chosen over ElevenLabs/Azure.

**No automatic failover.** Prata never switches backend silently. The active backend
is chosen deliberately and is always shown. If the active server is down → error tone, not a silent switch.
A backend that changes under you in an injection tool for patient data is a
patient-safety problem, not a convenience.

**Reuses Diktell's model and build chain.** whisper.cpp was chosen as the server
(not faster-whisper) for two reasons: (1) it loads exactly the same `ggml-model.bin`
that Diktell already uses — byte-identical model behavior, no
format conversion; (2) it reuses the CUDA build chain you already have in place
for Diktell.

## Verified topology (2026-06-16)

The table describes the actual deployment after room 4 (client) needed to
reach the GPU server in room 1. Use it as a reference when troubleshooting on new machines.

| Machine (Tailscale name) | LAN IP | Tailscale IP | Role | Key facts |
|---|---|---|---|---|
| **rum-ett** | `10.64.3.60` (static) | `100.80.217.12` | clinic PC, **GPU — server** | `whisper-server` port 8080, autostart at boot |
| **rum4-9700k** | `10.64.3.59` | `100.78.209.16` | clinic PC, **no GPU — client** | Runs Prata, backend LAN GPU-server |
| **ringvagen** | — | `100.87.6.56` | home PC (Windows 11), GPU — server | Backend Rngv GPU-server (Tailscale) |
| **ringvagen-wsl** | — | `100.115.64.39` | WSL on home PC | Cursor SSH, **not** in the transcription path |

**Chosen production path at the clinic:** rum4 → rum-ett **directly over LAN**
(`10.64.3.59 → 10.64.3.60`, same subnet, Ethernet). The patient audio therefore stays
within the hospital — no Tailscale relay involved. Tailscale is kept as a
fallback and for remote access at home.

**Tailscale is a mesh (tailnet), not pairwise connections.** Every machine
logged in with the same account becomes a node and reaches all the others via its `100.x` IP.
You don't "connect to" a single machine.

**Fallback if the LAN path is closed** (segmentation): point the Jobb backend to rum-ett's
Tailscale IP `100.80.217.12` instead of the LAN IP. In that case, check with
`tailscale status` that the connection shows as **direct** (not **relay**) — relay
means encrypted traffic goes out via a DERP relay outside the building. This is
an emergency fallback, not the production path (patient audio must stay on the LAN).

**Known warning (MagicDNS):** `tailscale status` may report that MagicDNS could
not set the DNS configuration ("the file is in use by another process"). Consequence:
use `100.x` IP addresses, not hostnames, until it is resolved. Does not block
the IP traffic.

**Measured latency (2026-06-16, rum-ett GPU):** approx. **1.4 s** per dictation. Compare
Berget Ai ~2.6 s (measured 2026-05-27).

### Two separate components — important to keep apart

The server consists of **two parts** that are handled in different ways:

| Component | What it is | How it gets onto the machine |
|---|---|---|
| **whisper.cpp** | The server program — open source C++ that receives HTTP calls and runs inference | Built from source in Step 1 (once) |
| **KB-Whisper-large** (`ggml-model.bin`) | The model — a pre-trained file from KBLab. It's what determines that you get Swedish and not generic Whisper | Downloaded (Option B) or already present via Diktell (Option A) |

So you do **not** "build" KB-Whisper — it is pre-trained by KBLab and downloaded
as a single ~3 GB file. It's the `-m` flag in Step 2 that points the
server program at the KB-Whisper file specifically. That guarantee lives in that command,
not in the build.

---

### Option A — Diktell already installed on the machine

**The build chain** (CUDA Toolkit, CMake, Visual Studio Build Tools 2022) is already
in place. Nothing extra to install.

**The KB-Whisper model** is already present — it is *exactly* the same `ggml-model.bin` as
Diktell loads. Find the path:

```powershell
Get-Content C:\Dev\diktell\config.toml | Select-String "model_path"
# typically: model_path = "models/ggml-model.bin"
# i.e. C:\Dev\diktell\models\ggml-model.bin
```

You can point the server directly at Diktell's model, or copy the file to your own
directory. On this machine there is a copy at
`C:\Dev\whisper-models\ggml-model.bin` — that is the path the autostart task and
the Step 2 command use. Change the path if you choose a different location.

**→ Go directly to Step 1.** The only thing missing is the server program whisper.cpp.

---

### Option B — new machine without Diktell

The server machine needs:

- An RTX 50 card (Blackwell, compute capability sm_120). Applies to both the 5070 Ti
  and the 5060 Ti.
- NVIDIA driver 570+ and CUDA Toolkit 12.8 or later (Blackwell requires
  at least 12.8).
- CMake 3.20+, Visual Studio Build Tools 2022 with the C++ workload, Git.

See Diktell's `docs/03-dev-environment.md` for the CUDA/CMake/Build
Tools installation (~30 min).

**Fetch KB-Whisper-large** from KBLab (~3 GB — grab a cup of coffee):

```powershell
New-Item -ItemType Directory -Path C:\Dev\whisper-models -Force
Invoke-WebRequest `
  -Uri "https://huggingface.co/KBLab/kb-whisper-large/resolve/main/ggml-model.bin" `
  -OutFile "C:\Dev\whisper-models\ggml-model.bin"
```

KB-Whisper-large is a Swedish Whisper variant trained by KBLab on Swedish texts.
Generic Whisper gives worse results on Swedish and is not used.

**→ Continue to Step 1.**

## Step 1 — Build whisper.cpp (the server program)

This step builds the **server program** whisper.cpp — not the model. The model
(`ggml-model.bin`) is already in place after Option A or B above. Step 1
only needs to be done once per machine:

```powershell
cd C:\Dev
git clone https://github.com/ggml-org/whisper.cpp
cd whisper.cpp
cmake -B build -DGGML_CUDA=1 -DCMAKE_CUDA_ARCHITECTURES=120 -DWHISPER_BUILD_EXAMPLES=1
cmake --build build -j --config Release
```

- `GGML_CUDA=1` enables the CUDA backend (the former flag `WHISPER_CUBLAS`
  is deprecated).
- `CMAKE_CUDA_ARCHITECTURES=120` = Blackwell/sm_120. Same value for the 5070 Ti
  and the 5060 Ti.
- The build takes a few minutes (compiles C++/CUDA).

Result: the server binary ends up in `build\bin\Release\whisper-server.exe`.

> ✅ *Verified 2026-06-15 on the RTX 5070 Ti (home PC).* The build passed with
> CUDA Toolkit 12.9 and Visual Studio Build Tools 2022 (`exit code 0`). Only
> harmless compiler warnings (floating-point and `size_t` conversion).
> The binary ended up at
> `C:\Dev\whisper.cpp\build\bin\Release\whisper-server.exe` and the CUDA backend
> finds the card at startup: *NVIDIA GeForce RTX 5070 Ti, compute capability
> 12.0, 16 GB VRAM*. `system_info` reports `CUDA : ARCHS = 1200` and
> `BLACKWELL_NATIVE_FP4 = 1` — the correct architecture was built.

## Step 2 — Start the server with KB-Whisper

The command points the server at **KB-Whisper-large** (`ggml-model.bin`) — exactly the same
model that Diktell loads. Adjust the `-m` path if you put the model in a different
location (see Prerequisites → Option A or B).

```powershell
C:\Dev\whisper.cpp\build\bin\Release\whisper-server.exe `
  -m C:\Dev\whisper-models\ggml-model.bin `
  --host 0.0.0.0 `
  --port 8080 `
  --inference-path /v1/audio/transcriptions `
  -l sv
```

- `-m` — path to **KB-Whisper-large** (`ggml-model.bin`). If you have Diktell
  you can also point directly at `C:\Dev\diktell\models\ggml-model.bin` — it's
  the same file.
- `--host 0.0.0.0` — listen on all network interfaces so other machines on
  the LAN can reach the server. The default `127.0.0.1` means *only this machine*.
- `--port 8080` — the server's port (default).
- `--inference-path /v1/audio/transcriptions` — makes the endpoint byte-identical
  to the OpenAI/Berget form Prata already sends. The default is `/inference`.
- `-l sv` — Swedish as the default language (the default is `en`). KB-Whisper is a
  Swedish model.

The server loads the model into VRAM once at startup and keeps it loaded —
no cold start per call (unlike Diktell's local cold start).

> ✅ *Verified 2026-06-15 on the RTX 5070 Ti.* At startup the model is loaded into CUDA0
> (`whisper_model_load: type = 5 (large v3)`, 3094 MB in VRAM) and the server
> starts listening on `0.0.0.0:8080`. The model file is ~2.9 GiB (≈3.1 GB), the same
> KB-Whisper-large as Diktell.

> ✅ *Verified 2026-06-16 on rum-ett (clinic).* The GPU process is visible in
> `nvidia-smi`, the endpoint responds at `http://10.64.3.60:8080/v1/audio/transcriptions`,
> and live dictation from rum4 gives approx. 1.4 s latency per call.

## Step 2b — Autostart the server at boot (home)

The server should behave like Tailscale: up after a reboot or power loss
*without* anyone logging in. It therefore runs as a scheduled task as
**SYSTEM, started at boot** (`AtStartup`) — not tied to your logon. It then
starts in session 0 at boot, needs no stored password (SYSTEM has
none), shows no console window (session 0 has no interactive desktop), and
gets auto-restart on crash.

**Register the task** (run as administrator — a UAC prompt appears):

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

- **SYSTEM / ServiceAccount / Highest** — runs at boot without logon, like a
  service, without a stored password. Since session 0 has no interactive desktop,
  the exe runs directly — no hide-window trick (VBScript) needed.
- **`AtStartup`** — triggers at boot, independent of logon. Survives
  reboot and power loss (comes up at the next boot, just like Tailscale).
- **`RestartCount 3` / `RestartInterval 1 min`** — if the server crashes it is restarted,
  up to three times one minute apart.
- **`ExecutionTimeLimit 0`** — no time limit; the server may run indefinitely.

> **GeForce + session 0:** the only technical uncertainty was whether CUDA works for
> SYSTEM in session 0 (no interactive desktop). It is verified that it
> does on this machine (see the note below) — pure GPU computation needs no
> desktop.

**Start now** without waiting for a reboot (requires administrator because the
task runs as SYSTEM — at a real boot the Task Scheduler starts it itself
without UAC):

```powershell
Start-ScheduledTask -TaskName PrataWhisperServer
```

**The computer must not fall asleep on its own.** Display sleep is harmless (the server
is unaffected), but if the whole machine goes into sleep/hibernate (S3/hibernate) the
GPU and the network stop — then the server is unreachable. Set the system timeouts to 0 on
AC power (the screen may well turn off):

```powershell
powercfg /change standby-timeout-ac 0     # the computer never sleeps on AC power
powercfg /change hibernate-timeout-ac 0
```

**Management** (start/stop/removal requires administrator because the task
is owned by SYSTEM):

```powershell
Get-ScheduledTask -TaskName PrataWhisperServer                          # status
Get-Process whisper-server | Select Id, StartTime                       # is it running?
Stop-ScheduledTask  -TaskName PrataWhisperServer                        # stop the server
Unregister-ScheduledTask -TaskName PrataWhisperServer -Confirm:$false   # remove autostart
```

> ✅ *Verified 2026-06-15 on the home PC.* The task runs as
> `NT AUTHORITY\SYSTEM`, port 8080 listens on `0.0.0.0`, and a
> transcription call against the SYSTEM-started instance returned correct JSON —
> i.e. **CUDA/GPU works in session 0**. The power settings are already correct
> (`standby-timeout-ac = 0`, `hibernate-timeout-ac = 0`), so the computer never sleeps
> on its own on AC power. Net result: the server starts at boot without
> logon and survives reboot/power loss — like Tailscale.

> ✅ *Verified 2026-06-16 on rum-ett (clinic).* The task **`PrataWhisperServer`**
> starts the server at boot without logon. See the firewall section under
> Step 4 — SYSTEM autostart **never** triggers the interactive firewall dialog,
> so an explicit inbound rule must be added manually.

> **Clinic:** the same task applies there, but follow the clinic's IT policy for
> services/autostart. The difference is the firewall rule (LAN, not Tailscale).

## Step 3 — Verify the server

**Locally on the server.** Check that the port is listening and make a small
test call with whisper.cpp's bundled sample audio (`samples\jfk.wav`):

```powershell
# is the port listening?
(Test-NetConnection 127.0.0.1 -Port 8080).TcpTestSucceeded   # -> True

# transcription call (jfk.wav is English, so set language=en just here)
curl.exe -X POST "http://127.0.0.1:8080/v1/audio/transcriptions" `
  -F "file=@C:\Dev\whisper.cpp\samples\jfk.wav" `
  -F "response_format=json" `
  -F "language=en"
```

The response should be OpenAI-compatible JSON, e.g.:

```json
{"text":" And so, my fellow Americans, ask not what your country can do for you, ask what you can do for your country."}
```

> ✅ *Verified locally 2026-06-15 on the RTX 5070 Ti.* The port responded and
> the endpoint returned correct JSON. Inference ~1.3 s for an 11-second
> clip on the GPU.

**Home — externally over Tailscale.** The home server is always reached externally (never
from a machine on the same physical home network), so the meaningful test is done from
an off-network device, e.g. a MacBook tethered to a phone. Replace `127.0.0.1` with
the server's **Tailscale IP** (`100.87.6.56`). curl is built into macOS:

```bash
curl -X POST "http://100.87.6.56:8080/v1/audio/transcriptions" \
  -F "file=@jfk.wav" \
  -F "response_format=json" \
  -F "language=en"
```

This proves the entire production path at home: off-network client → Tailscale → GPU.
Note: the test verifies *the server's reachability*. The Prata client itself (Step 5)
is Windows-only and does not run on the Mac — the Mac is a connectivity test, not
a dictation client.

> ⏳ *To be verified when the server is running and the MacBook is connected via Tailscale.*

**Clinic — internally on the clinic LAN.** At the clinic it's the opposite: dictation
happens from other workstations on the *same* network as the GPU machine, and patient audio
must never leave that network. Test from the client machine (e.g. rum4), **not just
locally on the server** — see the section "The server works locally but not from the
client" under Troubleshooting.

PowerShell **on the client machine** (e.g. rum4):

```powershell
Test-NetConnection 10.64.3.60 -Port 8080
# TcpTestSucceeded : True  ← required before Prata can dictate
```

> ✅ *Verified 2026-06-16.* From rum4 (`10.64.3.59`) to rum-ett (`10.64.3.60`):
> `TcpTestSucceeded : True` after the firewall rule was added on the server. Live dictation
> against LAN GPU-server works.

## Step 4 — Firewall rule (inbound)

Windows Defender does not let traffic into the server port until an inbound rule
has been added. **This is the most common error in a new deployment** — the server appears
to work on the GPU machine but is not reachable from clients (see Troubleshooting).

> **SYSTEM autostart and the firewall.** When `whisper-server` is started via the
> scheduled task (`PrataWhisperServer`, SYSTEM at boot), the interactive firewall dialog
> that pops up at manual start is **never** triggered. Inbound
> TCP 8080 is therefore blocked by default until you add an explicit rule —
> even though the server listens on `0.0.0.0:8080`.

Which rule depends on the *location* — and the two locations deliberately have different
access models:

| Location | Access | Rule |
|---|---|---|
| **Home** | Always external (Tailscale). No client sits on the same physical home network. | Tailscale rule only. |
| **Clinic** | Always internal on the clinic LAN. Patient audio never leaves the network. | LAN rule only (LocalSubnet). |

### Home — Tailscale rule

You reach the home server from your own devices (the country-house PC, MacBook) over Tailscale.
The rule is scoped to the tailnet range (`100.64.0.0/10`) so only your own
Tailscale devices can reach the port — not the internet at large, since the server's
`100.x` address is only reachable through the encrypted tunnel. Run as
administrator (a UAC prompt appears if the session is not elevated):

```powershell
New-NetFirewallRule `
  -DisplayName "Prata whisper-server (Tailscale)" `
  -Direction Inbound -Action Allow -Protocol TCP -LocalPort 8080 `
  -RemoteAddress 100.64.0.0/10 `
  -Profile Any
```

- `-RemoteAddress 100.64.0.0/10` — Tailscale's CGNAT range. The security guard
  is that the address is only reachable via the tunnel.
- `-Profile Any` — the Tailscale adapter's network profile varies; scoping on
  remote address is the actual guard, not the profile.
- Clients point at the home server's **Tailscale IP** (`100.87.6.56`), not the LAN IP.
  Further tightening can be done in Tailscale's own ACLs (which devices may
  reach port 8080).
- A LAN rule (`LocalSubnet`) is *not* needed at home — no client sits on
  the home network, so port 8080 is kept closed to the LAN for minimal exposure.

> ✅ *Verified 2026-06-15 on the home PC.* The rule is active: `Enabled = True`,
> `Inbound`, `Allow`, `TCP/8080`, `Profile = Any`,
> `RemoteAddress = 100.64.0.0/10`. (An earlier LocalSubnet rule was removed —
> at home only the external Tailscale path is used.) ⏳ A test call from an
> off-network device (MacBook via phone) over Tailscale is still pending.

### Clinic — LAN rule

At the clinic, dictation happens from other workstations on the clinic's *own* network,
and Tailscale is out of the question (patient audio must not leave the network). The rule is limited
to the local network:

```powershell
New-NetFirewallRule `
  -DisplayName "Prata whisper-server (LAN)" `
  -Direction Inbound -Action Allow -Protocol TCP -LocalPort 8080 `
  -RemoteAddress LocalSubnet `
  -Profile Domain
```

- `-RemoteAddress LocalSubnet` keeps access within the clinic network — no
  connections from outside.
- `-Profile Domain` is the likely profile on a clinic/region network.
  The firewall and port opening may, however, be controlled centrally by the clinic's IT — follow
  their policy instead of opening the port on your own.

**Diagnostic rule (during troubleshooting).** If you need to quickly confirm that the firewall
is the root cause, you can temporarily add a broader rule — run as administrator
**on the GPU server**:

```powershell
New-NetFirewallRule -DisplayName "Prata Whisper Server 8080" `
  -Direction Inbound -Protocol TCP -LocalPort 8080 -Action Allow -Profile Any
```

Then verify from the client machine (`Test-NetConnection 10.64.3.60 -Port 8080`).
Then tighten the rule (the server has no auth):

```powershell
Set-NetFirewallRule -DisplayName "Prata Whisper Server 8080" `
  -RemoteAddress 10.64.3.0/24
# or an explicit list: -RemoteAddress 10.64.3.59,10.64.3.61
```

> ⚠️ **Important — the netmask limits who can reach the server.** On the GPU machine the
> netmask is `255.255.255.192`. Windows `LocalSubnet` in the firewall rule above
> corresponds to the address range that belongs to that netmask on the server — not
> the entire clinic network. If the dictation workstations lie outside that
> range (likely if they have a different IP/netmask), they cannot reach the server. Then
> `-RemoteAddress` must be widened to the clients' address range instead of
> `LocalSubnet` — but keep it internal, never the internet.

> ✅ *Verified 2026-06-16 on rum-ett.* The root cause of rum4 not reaching the server
> was a **missing inbound rule**, not LAN segmentation (rum4 `10.64.3.59` and
> rum-ett `10.64.3.60` are on the same subnet). After the rule:
> `Test-NetConnection` from rum4 → `TcpTestSucceeded : True`, live dictation
> works. The clinic server *never* gets a Tailscale rule.

## Step 5 — Point Prata at the server

Prata has a **backend selector** with three options in the tray menu:

| Location (infra) | Tray ID (saved in `backend.txt`) | Display name in the menu |
|---|---|---|
| Home GPU (Tailscale) | `Hemma` | Rngv GPU-server (Tailscale) |
| Clinic GPU (LAN) | `Jobb` | LAN GPU-server |
| Cloud | `Berget` | Berget Ai |

Rngv and Rum1 are local whisper.cpp GPU servers (no auth); Berget Ai is the
cloud fallback (Bearer-authenticated). The ID is stable and saved in
`backend.txt`; the display names can be changed in code without breaking existing choices.

### Endpoint constants

The endpoints are hard-coded constants in `internal/transcribe/client.go`
(Prata follows "change a constant + recompile", no config file):

```go
const (
	HomeURL   = "http://100.87.6.56:8080/v1/audio/transcriptions" // home GPU via Tailscale
	WorkURL   = "http://10.64.3.60:8080/v1/audio/transcriptions"  // clinic GPU, static LAN IP
	BergetURL = "https://api.berget.ai/v1/audio/transcriptions"
)
```

- **HomeURL** points at the home server's **Tailscale IP**, so it works regardless of
  which network the client is on (cottage, mobile hotspot, home network).
- **WorkURL** points at the clinic server's **static LAN IP** (`10.64.3.60`). It is
  only reachable from inside the clinic network, so choosing LAN GPU-server outside the clinic gives
  an error tone — never a silent fallback. If the server's address changes: update the constant
  and rebuild.

### How the client finds the server (addressing)

`WorkURL` is a compiled-in constant — the address is baked into `prata.exe`. The server
listens on `0.0.0.0:8080` and responds regardless of whether it is reached via IP or name.

**Network settings at the clinic.** At every new computer installation, the
network is entered manually on the network card (IPv4, static configuration — not
DHCP). **DNS is the same on all machines**; **the IP and netmask vary per computer**
and are set based on which place in the network the machine is assigned. The GPU server is no
exception — it gets its own IP and subnet just like the other workstations.

| Value | GPU server (rum-ett) | Client (rum4) | Other computers |
|---|---|---|---|
| IP | `10.64.3.60` | `10.64.3.59` | different per machine |
| Netmask | `255.255.255.192` | same subnet | may vary per machine |
| DNS 1 | `192.44.242.131` | same | same |
| DNS 2 | `192.44.243.131` | same | same |

`WorkURL` points at the **GPU machine's** IP — not the client's. On this
installation:

```go
WorkURL = "http://10.64.3.60:8080/v1/audio/transcriptions"
```

If the GPU server changes IP (new machine, different netmask): update `WorkURL` to the new
address and rebuild Prata. The DNS addresses normally do not need to change.

The firewall rule (`LocalSubnet`) follows the server's netmask — see the warning under
Step 4 → Clinic if the dictation workstations have a different IP/netmask than the server.

Hostnames also work (shared DNS), e.g.
`http://RBS-PC:8080/v1/audio/transcriptions`, if the GPU machine is registered in
DNS. With manually set IPs per machine, it is easiest to use the IP directly in
`WorkURL`.

Two things to keep track of regardless of choice:

- `WorkURL` is a constant, so a *changed* address requires rebuilding Prata and
  redistributing it to the workstations.
- Client and server must be on the same LAN/subnet that the firewall rule
  (`LocalSubnet`) allows — if the clients are on a different subnet/VLAN, the
  rule must be widened, which the clinic's IT controls in that case.

### Conditional auth

Only Berget Ai sends `Authorization: Bearer <key>`. The local GPU servers
never get the key. The API key is now loaded "best-effort": if it is missing, Prata
still starts (local backends need none), but Berget Ai reports an error if it
is selected without a key.

### Choosing a backend

Right-click the tray icon → choose **Rngv GPU-server (Tailscale) / LAN GPU-server / Berget Ai**
(radio buttons, the active choice is marked). The active backend is always shown:

- in the tray tooltip (`Prata — Rngv GPU-server (Tailscale)`), and
- as a balloon when you switch (`Aktiv transkribering: Rngv GPU-server (Tailscale)`).

The choice is saved as a stable ID (`Hemma`, `Jobb`, or `Berget`) in
`%LOCALAPPDATA%\Prata\backend.txt` and survives reboot. **The default on first
start** (missing or invalid `backend.txt`) is **LAN GPU-server** (`Jobb`) —
the internal GPU server on the clinic LAN, which requires no API key. Choose
**Rngv GPU-server (Tailscale)** at home or **Berget Ai** in the menu as needed.

**No silent failover:** if the chosen server is down you get an error tone when dictating, not
a silent switch to another backend. A switch happens only when *you* choose in the menu.

### Testing

**Clinic (rum4 → rum-ett):**

1. Confirm that the GPU server is running on rum-ett (`Get-Process whisper-server` or
   that the scheduled task `PrataWhisperServer` is running).
2. From rum4: `Test-NetConnection 10.64.3.60 -Port 8080` → `TcpTestSucceeded : True`.
3. Run Prata on rum4. Right-click the tray icon → **LAN GPU-server**.
4. Hold **F1**, dictate, release — the text is transcribed against the GPU server (~1.4 s).

**Home (Tailscale):**

1. Start the server on the home PC (the Step 2 command or autostart).
2. Run Prata. Right-click the tray icon → **Rngv GPU-server (Tailscale)**.
3. Hold **F1**, dictate, release.

Quick reachability test without Prata (e.g. from a MacBook over Tailscale) — see
Step 3.

> ✅ *Implemented and verified 2026-06-15:* builds cleanly (`go build`,
> `go vet`), unit tests for backend routing and conditional auth pass.
> ✅ *Live dictation verified 2026-06-16:* rum4 → rum-ett (LAN GPU-server,
> LAN, ~1.4 s latency). ⏳ Live dictation against Rngv GPU-server (Tailscale) is still pending
> as a separate test.

## Installation prompt — clinic PC (paste into Cursor/Claude)

The idea behind the clinic deployment: check out the Prata repo on the clinic machine (this
document comes with it), open Cursor/Claude in the repo folder, and paste the prompt
below. The agent runs the entire server setup for the **clinic scenario** (LAN-only) and
stops only where you need to approve (UAC) or make a decision. Everything the agent
needs is in this document.

```text
You are setting up the Prata GPU server on a CLINIC PC (clinic). The full guide is in
PRATA-GPU-SERVER.md in this folder — read it first and follow it. Run from
the root of the Prata repo (where the document lives). This is the CLINIC scenario, not home:

- LAN only. Tailscale is OUT OF THE QUESTION. Patient audio must NEVER leave the clinic network.
- The firewall is scoped to LocalSubnet/Domain — NEVER add a Tailscale rule.
- Leave HomeURL untouched. Do not touch any home-specific parts.

Work autonomously. Stop only for (a) UAC approvals and (b) decisions that require
me. Combine ALL elevated steps into ONE single elevated call so I only need to
approve one (1) UAC prompt. Verify each step before moving on; report
briefly what was done.

Steps:

1. PREREQUISITES. Run `nvidia-smi`, determine the GPU model and compute
   capability and set `CMAKE_CUDA_ARCHITECTURES` accordingly (RTX 50 series =
   Blackwell sm_120 -> 120; other card -> look up the correct arch number). Check
   that CUDA Toolkit (>=12.8 for Blackwell), CMake 3.20+, and Visual Studio Build
   Tools 2022 are present. If the machine already builds Diktell the chain is in place; otherwise
   follow the Prerequisites section. Download the model (`ggml-model.bin`, the same
   as Diktell) if it is missing.

2. BUILD. Follow Step 1 (whisper.cpp with CUDA) with the correct arch value. Verify that
   `whisper-server.exe` was built and that the CUDA backend finds the card.

3. NETWORK CARD. The network is entered manually on the network card (IPv4, static).
   DNS is always `192.44.242.131` and `192.44.243.131` (the same on all machines).
   IP and netmask vary per computer — on the GPU server (this machine) it is
   IP `10.64.3.60`, netmask `255.255.255.192`. Confirm with `ipconfig` that the
   settings are correct; note the IP — it becomes `WorkURL`.

4. ONE ELEVATED CALL (one UAC) that does everything below in sequence:
   a. Firewall: add the LAN rule from "Step 4 -> Clinic". If unsure, use the
      diagnostic rule (-Profile Any) first and verify from the client machine with
      Test-NetConnection; then tighten with -RemoteAddress. NOTE: SYSTEM autostart
      NEVER triggers the firewall dialog — the rule must be added explicitly. If
      LocalSubnet is not enough (client outside the server's subnet), stop and ask
      me for the correct address range. If the rule is blocked or governed by GPO -> stop
      and ask me to involve the clinic's IT.
   b. Power mode: `powercfg /change standby-timeout-ac 0` and
      `powercfg /change hibernate-timeout-ac 0` so the machine does not sleep (the screen
      may turn off). If this is governed by GPO -> note it.
   c. Autostart: register the SYSTEM/boot task EXACTLY as in Step 2b
      (AtStartup, ServiceAccount/Highest, restart 3x1 min, ExecutionTimeLimit 0).
      If the clinic's IT controls services/autostart centrally -> check with them.
   d. Start the task (Start-ScheduledTask).

5. VERIFY. Wait for the model load, confirm that the process runs as
   NT AUTHORITY\SYSTEM, that port 8080 listens on 0.0.0.0. Test from ANOTHER
   workstation on the LAN (not just locally on the server). Test-NetConnection
   against the server's LAN IP should give TcpTestSucceeded : True. Make a transcription call
   (Step 3).

6. POINT PRATA AT THE SERVER. `WorkURL` is already set in
   `internal/transcribe/client.go` to `http://10.64.3.60:8080/v1/audio/transcriptions`
   (the clinic server's static IP). Confirm that the IP is correct; only change it if the server
   got a different address. Rebuild Prata (`go build …`). Note that the rebuilt
   `prata.exe` must be distributed to the workstations that will
   dictate against the server. Verify that the LAN GPU-server backend can be selected and dictates.

7. FINAL REPORT. Update the clinic status lines in PRATA-GPU-SERVER.md and give me:
   GPU/arch, model path, firewall rule, autostart status, verification result,
   and which WorkURL was set.
```

## Troubleshooting

### The server works locally but not from the client

**Symptom:** the GPU server responds when you open `http://10.64.3.60:8080` *on
the server machine*, but the client (e.g. rum4) gets a timeout or Prata plays an
error tone when dictating.

**Why it looks confusing:** local access to the machine's own IP goes via
loopback and **never passes the Windows firewall**. A connection from another
machine does. The server "working locally" therefore does *not* prove that
clients can reach it — only that the process is listening.

**Troubleshooting order:**

| Step | Hypothesis | Command / action | Typical outcome |
|---|---|---|---|
| 1 | Server down | `Get-Process whisper-server` on the GPU machine | Process missing → start the task |
| 2 | Wrong port | `Test-NetConnection 10.64.3.60 -Port 8080` from the client | The server listens on **8080**, not 8000 |
| 3 | Bound to localhost | Test `10.64.3.60` locally *on the server* | Works locally but not from the client → firewall, not binding |
| 4 | LAN segmentation | `Test-NetConnection` from the client, look at `SourceAddress` | Same subnet (e.g. `10.64.3.59 → 10.64.3.60`) → segmentation ruled out |
| 5 | **Firewall** | Add an inbound rule (Step 4 → Clinic) | **Most common root cause** with SYSTEM autostart |

**Verification after the firewall fix** (PowerShell on the client machine):

```powershell
Test-NetConnection 10.64.3.60 -Port 8080
# TcpTestSucceeded : True
```

> ✅ *Verified 2026-06-16:* rum4 could not reach rum-ett even though the server responded
> locally. Root cause: a missing inbound rule. After the rule: live dictation works.

### Verify that KB-Whisper-Large is running

KB-Whisper-Large is a fine-tune of OpenAI's whisper-large — identical
architecture and vocabulary. The model's internal metadata does **not** reveal whether it is
the KB version. Instead, verify in two ways:

1. **Origin (identity).** Check which `.bin` the server was started with:

   ```powershell
   Get-CimInstance Win32_Process -Filter "Name='whisper-server.exe'" |
     Select-Object -ExpandProperty CommandLine
   ```

   Look at the `-m`/`--model` argument. A `ggml-model.bin` from
   `KBLab/kb-whisper-large` is correct; a generic `ggml-large-v3.bin` from
   whisper.cpp's default download is the wrong model.

2. **Behavior (clinical confirmation).** Dictate the same Swedish sentence with medical
   terms against **LAN GPU-server** and **Berget Ai** (confirmed
   KB-Whisper-Large) respectively and compare. An identical error pattern ⇒ the same model, and
   `dictionary-corrections.txt` (the baseline is embedded in the binary; the per-user override file
   lives in `%LOCALAPPDATA%\Prata\`) is correctly calibrated.

### Prata client installation — known issues

- **AV/EDR blocking:** an unsigned `prata.exe` can be blocked by Webroot/SmartScreen
  on new machines (loader error without a crash). Allowlist `%ProgramFiles%\Prata`
  (machine-wide install), or sign the binary.
- **Machine-wide install (recommended at the clinic):** copy `prata.exe` from USB
  and run `prata.exe --install` (UAC). The binary ends up in
  `%ProgramFiles%\Prata\`, autostart is registered machine-wide for all users.
  Per-user data (`apikey.dat`, `backend.txt`, dictionary override) is created
  automatically under `%LOCALAPPDATA%\Prata\` as needed. Hardware smoke test
  deferred — see PRATA-DESIGN-LOG.md (2026-06-17).
- **Build yourself / offline:** for machines without a prebuilt binary — build `prata.exe`
  from source (`go build …`, requires Go + a C toolchain) and then run
  `prata.exe --install`. If you have a prebuilt binary: copy `prata.exe`
  (+ the `.bat` wrappers) from USB and run `--install`.
- **Autostart:** the scheduled task **`Prata`** starts the client at
  logon (distinct from the server task **`PrataWhisperServer`** on the GPU machine).
  Machine-wide install: one task for all users (RunLevel Limited).

### Open security points

- **Tighten the firewall rule** on the GPU server: switch from `-Profile Any` to
  `-RemoteAddress` with known client IPs (see Step 4 → Clinic).
- **Rotate the Berget key** if it was exposed in cleartext during troubleshooting; run
  `prata.exe --set-key` again on the affected machines.

## Status

| Step | Status |
|---|---|
| Architecture & principles | Done |
| Prerequisites | Done |
| Step 1 — Build whisper.cpp | ✅ Verified on the RTX 5070 Ti (home PC) |
| Step 2 — Start the server | ✅ Verified — model loaded on the GPU, listening |
| Step 2b — Autostart (home) | ✅ Runs as SYSTEM at boot — survives reboot/power loss, GPU verified in session 0 |
| Step 3 — Verify | ✅ Locally + LAN from rum4→rum-ett (2026-06-16); ⏳ Tailscale test (home) remaining |
| Step 4 — Firewall (home/Tailscale) | ✅ Rule active on the home PC (ringvagen) |
| Step 4 — Firewall (clinic/LAN) | ✅ Verified on rum-ett (2026-06-16); root cause on failure: missing inbound rule |
| Step 5 — Prata client | ✅ Live dictation rum4→rum-ett (~1.4 s); LAN GPU-server production path; machine-wide `--install` implemented (smoke test deferred) |
