// Command prata runs the full push-to-talk loop: F1 held →
// microphone capture; release → encode, transcribe, correct, and inject
// the text into the foreground window. Quit via the system-tray "Avsluta"
// menu (or Ctrl+C when run from a terminal).
//
// The API key comes from the BERGET_API_KEY environment variable, or a
// DPAPI-encrypted file written by `prata --set-key` (see internal/auth).
//
// Usage:
//
//	prata                    run the daemon
//	prata --set-key <key>    encrypt and store the Berget API key, then exit
package main

import (
	"bytes"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"

	"github.com/carlosriveros/prata/internal/audio"
	"github.com/carlosriveros/prata/internal/auth"
	"github.com/carlosriveros/prata/internal/cue"
	"github.com/carlosriveros/prata/internal/daemonlog"
	"github.com/carlosriveros/prata/internal/dict"
	"github.com/carlosriveros/prata/internal/hotkey"
	"github.com/carlosriveros/prata/internal/icon"
	"github.com/carlosriveros/prata/internal/inject"
	"github.com/carlosriveros/prata/internal/installer"
	"github.com/carlosriveros/prata/internal/popup"
	"github.com/carlosriveros/prata/internal/sanity"
	"github.com/carlosriveros/prata/internal/single"
	"github.com/carlosriveros/prata/internal/transcribe"
	"github.com/carlosriveros/prata/internal/tray"
	"github.com/carlosriveros/prata/internal/ui"
	"github.com/carlosriveros/prata/internal/update"
)

// version is the release this binary was built from. The release workflow
// injects the git tag via -ldflags "-X main.version=…"; a plain
// `go build`/`go run` leaves it as "dev", which never reports an available
// update against a real release tag.
var version = "dev"

// event is what the listener enqueues for the processor goroutine.
// Using a typed enum keeps the channel small and self-documenting.
type event int

const (
	evPress event = iota
	evRelease
)

// dictAdd is a correction rule captured by the F8 quick-fix flow and
// handed to the processor goroutine, which owns the *dict.Dict and is the
// only goroutine allowed to Save/Reload it — so no lock is needed.
type dictAdd struct {
	wrong, correct string
}

// f8Busy is the single-flight guard for the F8 quick-fix flow: at most one
// grab → popup → paste-back runs at a time. A second F8 tap while one is in
// flight is dropped.
var f8Busy atomic.Bool

// inputMu serializes all foreground, clipboard, and SendInput work across
// PTT injection and F8 quick-fix. Without this gate, an async transcription
// result can interleave with F8's Ctrl+C/Ctrl+V or steal focus from the popup.
var inputMu sync.Mutex

// minCaptureBytes is the smallest PCM payload worth transcribing,
// roughly 0.1s of audio. Derived from the transcribe format constants
// so it tracks the sample rate.
const minCaptureBytes = transcribe.SampleRate * transcribe.NumChannels * transcribe.BitsPerSample / 8 / 10

// transcribeJob carries a finished audio capture to the worker. targetHwnd
// is the foreground window captured when F1 was pressed; the result must
// return to that same window before injection, because async transcription
// may finish after the user has focused something else.
type transcribeJob struct {
	pcm        []byte
	targetHwnd uintptr
}

// transcribeResult carries a finished transcription from the worker
// goroutine back to the processor. Keeping the blocking Berget call off the
// processor means a slow response never freezes F1 capture.
type transcribeResult struct {
	text       string
	elapsed    time.Duration
	err        error
	targetHwnd uintptr
}

// transcribeQueueDepth bounds how many finished captures can wait for
// transcription. A single slow Berget round (~24s observed) must not freeze
// capture, but an unbounded queue under a sustained outage would hide the
// failure and pile up stale audio; past this depth the processor drops the
// capture with an error cue so the user re-dictates instead.
const transcribeQueueDepth = 8

// loadDict builds the active dictionary: the embedded baseline with the
// per-user override (PRATA_DICT_PATH or %LOCALAPPDATA%\Prata\...) layered on
// top. Path resolution lives entirely in the dict package (LoadDefault), so
// loadDict and dict.Save/Reload can never disagree about where the override
// is. A nil return paired with a non-nil error means dict corrections are
// disabled but the app should still run; in practice the embedded baseline
// loads even when no override file exists.
func loadDict() (*dict.Dict, error) {
	return dict.LoadDefault()
}

// backendPrefPath is where the active backend choice is stored:
// %LOCALAPPDATA%\Prata\backend.txt. This is state (the last deliberate
// choice), not config — it is written by the tray selection, not hand-edited,
// which is why it lives next to apikey.dat rather than being a constant.
func backendPrefPath() string {
	return filepath.Join(os.Getenv("LOCALAPPDATA"), "Prata", "backend.txt")
}

// loadBackendPref reads the persisted backend ID and resolves it to a
// Backend, falling back to Work (the local "Jobb" GPU server) when the file
// is missing/unreadable or names an unknown backend. Work is the safe default
// for a fresh install: a local GPU server needs no API key, so a new clinic
// user with no backend.txt lands on a working transcriber instead of
// Berget-without-a-key, which would only surface as an error cue on the first
// F1. A broken or foreign backend.txt falls back the same way and never
// silently to Berget (see PRATA-DESIGN-LOG, decisions 5 and 8). This default
// is hard-coded — one binary, no separate build, no ldflags.
func loadBackendPref() transcribe.Backend {
	data, err := os.ReadFile(backendPrefPath())
	if err != nil {
		return transcribe.Work
	}
	if b, ok := transcribe.BackendByName(strings.TrimSpace(string(data))); ok {
		return b
	}
	return transcribe.Work
}

// saveBackendPref persists the chosen backend by stable ID. Failures are logged,
// not fatal: a write error just means the choice will not survive a restart.
func saveBackendPref(b transcribe.Backend) {
	path := backendPrefPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		fmt.Fprintf(os.Stderr, "backend pref mkdir: %v\n", err)
		return
	}
	if err := os.WriteFile(path, []byte(b.ID+"\n"), 0o600); err != nil {
		fmt.Fprintf(os.Stderr, "backend pref write: %v\n", err)
	}
}

// dispatchSubcommand handles one-shot CLI subcommands that must run instead
// of the daemon. It parses args manually rather than via the flag package,
// whose usage/error output goes to stderr — invisible under -H windowsgui.
// All feedback is shown through a message box. It returns true when a
// subcommand was handled and the caller should exit without starting the
// daemon. This is maintenance-path code, deliberately separate from the
// dictation hot path.
func dispatchSubcommand(args []string) bool {
	if len(args) == 0 {
		return false
	}
	switch args[0] {
	case "--set-key":
		runSetKey(args[1:])
		return true
	case "--install":
		installer.Run()
		return true
	case "--uninstall":
		installer.Uninstall()
		return true
	default:
		return false
	}
}

// runSetKey encrypts the supplied Berget API key with user-scope DPAPI and
// writes it to %LOCALAPPDATA%\Prata\apikey.dat via auth.SaveAPIKey, then
// reports the outcome in a message box. Pure argument form
// (`prata --set-key <key>`); there is no interactive prompt because
// windowsgui builds have no stdin. It writes per-user at medium integrity and
// never elevates.
func runSetKey(args []string) {
	if len(args) == 0 || strings.TrimSpace(args[0]) == "" {
		ui.MessageBox("Prata", "Kunde inte spara nyckeln: ingen nyckel angiven.", ui.IconError)
		return
	}
	key := strings.TrimSpace(args[0])
	if err := auth.SaveAPIKey(key); err != nil {
		ui.MessageBox("Prata", fmt.Sprintf("Kunde inte spara nyckeln: %v.", err), ui.IconError)
		return
	}
	ui.MessageBox("Prata", "Nyckeln sparad.", ui.IconInfo)
}

func main() {
	// One-shot subcommands (e.g. --set-key) run instead of the daemon and
	// exit. Dispatch before any daemon setup (DPI, single-instance, audio) so
	// a maintenance command never trips the "already running" guard or spins
	// up subsystems it doesn't need.
	if dispatchSubcommand(os.Args[1:]) {
		return
	}

	// Per-monitor DPI awareness must be set before any window or HICON is
	// created (the tray icon below), so it renders crisp on scaled displays.
	tray.SetProcessDPIAware()

	// Refuse to start if another Prata is already running. Two instances
	// share F1 and would both capture and inject, producing
	// duplicate output (or garbled output in async target apps).
	if !single.Acquire("PrataSingleInstanceMutex") {
		fmt.Fprintln(os.Stderr, "Prata is already running; exiting.")
		return
	}

	// The Berget API key is loaded best-effort: it is only needed when the
	// Berget backend is active. The local GPU backends (Hemma / Jobb) need
	// no key, so a missing key no longer prevents startup — it just means
	// the Berget backend will report an error if selected without a key.
	apiKey := os.Getenv("BERGET_API_KEY")
	if apiKey == "" {
		if k, err := auth.LoadAPIKey(); err == nil {
			apiKey = k
		}
	}
	if apiKey == "" {
		fmt.Fprintln(os.Stderr, "no Berget API key found; the Berget backend will not work")
		fmt.Fprintln(os.Stderr, "  (set BERGET_API_KEY or run prata --set-key); local GPU backends still work")
	}

	d, err := loadDict()
	if err != nil {
		fmt.Fprintf(os.Stderr, "warn: dictionary disabled (%v)\n", err)
		// d will be nil here; processEvents handles nil gracefully
	} else {
		fmt.Fprintln(os.Stderr, "dictionary loaded")
	}

	// Resolve the active transcription backend from the persisted choice
	// (default Berget). The selection is changed deliberately in the tray and
	// never switched silently.
	active := loadBackendPref()
	client := transcribe.NewClient(apiKey)
	client.SetBackend(active)
	fmt.Fprintf(os.Stderr, "active backend: %s (%s)\n", active.DisplayName, active.URL)

	// Buffered so the listener's message-loop goroutine never blocks.
	// Size 4 covers any realistic press/release burst.
	events := make(chan event, 4)

	listener := hotkey.NewListener(
		func() { events <- evPress },
		func() { events <- evRelease },
	)

	// F8 quick-fix: grab the foreground selection, let the user correct it
	// in a popup, persist the rule, and paste the correction back. The dict
	// save+reload is handed over dictAdds to the processor goroutine, which
	// owns d. SetOnF8 must run before listener.Run for the same
	// happens-before guarantee the press/release callbacks rely on.
	dictAdds := make(chan dictAdd, transcribeQueueDepth)
	listener.SetOnF8(func() {
		// Listener goroutine: must return quickly to keep the message
		// loop responsive. Do only the cheap, time-critical work (grab
		// the foreground HWND before focus changes) and hand the rest
		// to a goroutine.
		if !f8Busy.CompareAndSwap(false, true) {
			return // a quick-fix is already in flight; drop this tap
		}
		sourceHwnd, ok := inject.ForegroundWindow()
		if !ok {
			f8Busy.Store(false)
			cue.PlayError()
			return
		}
		go f8Worker(sourceHwnd, dictAdds)
	})

	// Listener goroutine: pins itself to its OS thread and runs the
	// Windows message loop until Stop is called.
	listenerDone := make(chan error, 1)
	go func() {
		listenerDone <- listener.Run()
	}()

	// Transcription worker: the only goroutine that makes the blocking
	// Berget call. Decoupling it from the processor keeps F1 capture
	// responsive even when one request is slow (~24s seen during a Berget
	// hiccup). A single worker draining a FIFO queue preserves dictation
	// order — injected text always matches the order spoken.
	jobs := make(chan transcribeJob, transcribeQueueDepth)
	results := make(chan transcribeResult, transcribeQueueDepth)
	stopTranscription := make(chan struct{})
	go func() {
		defer close(results)
		for {
			select {
			case <-stopTranscription:
				return
			case job, ok := <-jobs:
				if !ok {
					return
				}
				start := time.Now()
				text, terr := client.Transcribe(bytes.NewReader(transcribe.EncodePCM(job.pcm)))
				result := transcribeResult{
					text:       text,
					elapsed:    time.Since(start),
					err:        terr,
					targetHwnd: job.targetHwnd,
				}
				select {
				case results <- result:
				case <-stopTranscription:
					return
				}
			}
		}
	}()

	// Open the per-day daemon log before the processor starts emitting events
	// to it. Best-effort: under -H windowsgui stderr is discarded, so this file
	// is the only durable record — but a missing log must never be fatal, so a
	// failure just falls back to stderr and continues. Closed at shutdown.
	if closer, err := daemonlog.Open(); err != nil {
		fmt.Fprintf(os.Stderr, "daemonlog: %v (continuing without file log)\n", err)
	} else {
		defer closer.Close()
	}

	// Processor goroutine: drains events sequentially, owning the
	// audio.Session lifecycle, and applies + injects finished
	// transcriptions. Single-goroutine ownership means no mutex is needed on
	// the session pointer or the dictionary.
	processorDone := make(chan struct{})
	go func() {
		defer close(processorDone)
		processEvents(client, d, events, dictAdds, jobs, results, stopTranscription)
	}()

	// System-tray icon. Its only menu item, Avsluta, requests shutdown by
	// closing quit. onQuit runs on the tray's UI thread, must return fast,
	// and must not call t.Stop() (the tray posts its own WM_QUIT) — it only
	// nudges the shared shutdown path below. quitOnce makes repeat Avsluta
	// clicks harmless. In production (-H windowsgui, no console) Avsluta is
	// the only graceful quit path, since Ctrl+C is never delivered.
	quit := make(chan struct{})
	var quitOnce sync.Once
	t := tray.New(icon.ICO, "Prata", func() {
		quitOnce.Do(func() { close(quit) })
	})
	// "Sök efter uppdatering…": notify-only update check. Must return fast
	// on the tray UI thread, so the network call runs on its own goroutine
	// and reports back via a tray balloon. Set before t.Run is launched so
	// the menu is built with the item present.
	t.SetOnCheckUpdate(func() {
		go checkForUpdate(t)
	})
	// Backend selector (Rngv GPU-server (Tailscale) / LAN GPU-server / Berget Ai) as radio
	// items in the tray menu, with the persisted choice pre-selected. The saved
	// ID (Hemma/Jobb/Berget) is stable; display names can change without breaking
	// backend.txt. Switching is deliberate and visible: the tray updates the
	// tooltip and shows a balloon, the new choice is persisted, and the client
	// routes the next dictation there. Must be set before t.Run is launched.
	backendNames := make([]string, len(transcribe.Backends))
	activeIdx := 0
	for i, b := range transcribe.Backends {
		backendNames[i] = b.DisplayName
		if b.ID == active.ID {
			activeIdx = i
		}
	}
	t.SetBackends(backendNames, activeIdx)
	t.SetOnSelectBackend(func(idx int) {
		b := transcribe.Backends[idx]
		client.SetBackend(b)
		go saveBackendPref(b)
		// Swedish user feedback, with a caveat when the chosen backend can't
		// actually serve yet (Berget without a key, or an unconfigured Work
		// server). It still switches — Prata never overrides a deliberate
		// choice — but the user is told why a dictation may fail.
		switch {
		case b.RequiresKey && apiKey == "":
			t.Notify("Prata", fmt.Sprintf("Aktiv transkribering: %s. Varning: ingen API-nyckel. Kör prata --set-key.", b.DisplayName))
		case b.URL == "":
			t.Notify("Prata", fmt.Sprintf("Aktiv transkribering: %s. Servern är inte konfigurerad än.", b.DisplayName))
		default:
			t.Notify("Prata", fmt.Sprintf("Aktiv transkribering: %s", b.DisplayName))
		}
		fmt.Fprintf(os.Stderr, "backend switched: %s (%s)\n", b.DisplayName, b.URL)
	})
	trayDone := make(chan error, 1)
	go func() {
		trayDone <- t.Run()
	}()
	trayAlive := true

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt)

	fmt.Fprintln(os.Stderr, "PTT ready. Hold F1 to dictate. Ctrl+C to quit.")

	// shutdown is the teardown shared by Ctrl+C and tray Avsluta: stop the
	// listener, drain the processor, then stop the tray — but only if the
	// tray is still running, since one that failed to start has already
	// returned (and its trayDone has been niled below).
	shutdown := func() {
		listener.Stop()
		<-listenerDone
		close(events)
		<-processorDone
		if trayAlive {
			t.Stop()
			<-trayDone
		}
	}

	for {
		select {
		case <-sigs:
			shutdown()
			return
		case <-quit:
			shutdown()
			return
		case err := <-listenerDone:
			// Listener returned on its own; tear down the rest and exit.
			close(events)
			<-processorDone
			if trayAlive {
				t.Stop()
				<-trayDone
			}
			if err != nil {
				fmt.Fprintf(os.Stderr, "listener error: %v\n", err)
				os.Exit(1)
			}
			return
		case err := <-trayDone:
			// Tray Run returned. A non-nil error is a fundamental setup
			// failure; the icon is only a convenience, so log it and keep
			// dictating — the same soft-degrade policy as the dictionary.
			// Nil the channel so this case can't re-fire, and mark the tray
			// dead so shutdown skips waiting on it.
			trayAlive = false
			trayDone = nil
			if err != nil {
				fmt.Fprintf(os.Stderr, "warn: tray disabled (%v)\n", err)
			}
		}
	}
}

// processEvents drains the event channel sequentially, managing the
// audio.Session lifecycle and handing finished captures to the transcription
// worker over jobs. Transcribed text comes back over results, where the
// dictionary is applied and the text injected — all on this one goroutine,
// so d needs no lock. Transcription itself runs on the worker, so a slow
// Berget response never blocks the next F1 press.
//
// Defensive: ignores duplicate press while already recording, and
// release without an active session. With the current state machine in
// internal/hotkey these can't fire, but the cost of the guard is
// trivial and protects against future listener-state regressions.
//
// client is read only for the active backend ID stamped into each daemonlog
// line; the transcription itself still runs on the worker goroutine.
func processEvents(client *transcribe.Client, d *dict.Dict, events <-chan event, dictAdds <-chan dictAdd, jobs chan<- transcribeJob, results <-chan transcribeResult, stopTranscription chan struct{}) {
	var session *audio.Session
	var targetHwnd uintptr

	for {
		select {
		case ev, ok := <-events:
			if !ok {
				// Shutdown: close(events) ends the loop. Stop the worker too
				// so an in-flight Berget result cannot block forever trying
				// to send to a processor that has already exited.
				close(stopTranscription)
				close(jobs)
				return
			}
			switch ev {
			case evPress:
				if session != nil {
					continue
				}
				targetHwnd = 0
				if hwnd, ok := inject.ForegroundWindow(); ok {
					targetHwnd = hwnd
				}
				s, err := audio.Start()
				if err != nil {
					fmt.Fprintf(os.Stderr, "audio start: %v\n", err)
					cue.PlayError()
					targetHwnd = 0
					continue
				}
				session = s
				cue.PlayStart()
				fmt.Fprintln(os.Stderr, "recording...")
				daemonlog.Printf("recording backend=%s", client.ActiveBackend().ID)

			case evRelease:
				if session == nil {
					continue
				}
				pcm, err := session.Stop()
				session = nil
				hwnd := targetHwnd
				targetHwnd = 0
				if err != nil {
					fmt.Fprintf(os.Stderr, "audio stop: %v\n", err)
					cue.PlayError()
					continue
				}
				cue.PlayStop()

				// An empty / near-empty capture (e.g. an accidental brief
				// tap) would otherwise be sent to Berget and block for the
				// full 30s HTTP timeout before failing. Skip it instead.
				if len(pcm) < minCaptureBytes {
					fmt.Fprintln(os.Stderr, "no audio captured, skipping")
					daemonlog.Printf("capture too short, skipping")
					continue
				}

				// Hand the capture to the transcription worker instead of
				// transcribing inline, so a slow Berget response never
				// freezes the next F1 press. Non-blocking: if the queue is
				// full (sustained outage) drop with an error cue rather than
				// stalling capture or piling up stale audio.
				select {
				case jobs <- transcribeJob{pcm: pcm, targetHwnd: hwnd}:
					fmt.Fprintf(os.Stderr, "captured %d bytes, transcribing...\n", len(pcm))
				default:
					fmt.Fprintln(os.Stderr, "transcription queue full, dropping capture")
					daemonlog.Printf("transcription queue full, dropping")
					cue.PlayError()
				}
			}

		case res, ok := <-results:
			if !ok {
				results = nil
				continue
			}
			// A finished transcription from the worker: apply corrections,
			// guard against degenerate output, and inject. Production runs
			// -H windowsgui (no console), so the error cue is the only
			// failure signal on each discard path below.
			backendID := client.ActiveBackend().ID
			elapsed := res.elapsed.Seconds()
			if res.err != nil {
				fmt.Fprintf(os.Stderr, "transcribe: %v\n", res.err)
				daemonlog.Printf("transcribe error backend=%s elapsed=%.2fs err=%v", backendID, elapsed, res.err)
				cue.PlayError()
				continue
			}
			text := res.text
			if d != nil {
				text = d.Apply(text)
			}
			// Empty / whitespace-only result (e.g. a short capture with
			// no clear speech) would otherwise inject a bare newline.
			if strings.TrimSpace(text) == "" {
				fmt.Fprintf(os.Stderr, "empty transcription, skipping (%.2fs)\n", res.elapsed.Seconds())
				daemonlog.Printf("empty transcription backend=%s elapsed=%.2fs", backendID, elapsed)
				cue.PlayError()
				continue
			}
			// A Whisper repetition loop (common on long digit strings)
			// would otherwise be injected verbatim into the patient
			// journal — a safety hazard, not just noise. Discard it and
			// log a prefix so the dropped text stays visible and the
			// user can re-dictate.
			if sanity.IsDegenerate(text) {
				fmt.Fprintf(os.Stderr, "discarded degenerate transcription (ratio %.1f), skipping: %q\n", sanity.Ratio(text), preview(text, 80))
				daemonlog.Printf("degenerate ratio=%.1f backend=%s elapsed=%.2fs", sanity.Ratio(text), backendID, elapsed)
				cue.PlayError()
				continue
			}
			if !strings.HasSuffix(text, "\n") {
				text += "\n"
			}
			if res.targetHwnd == 0 {
				fmt.Fprintln(os.Stderr, "inject target missing, skipping")
				daemonlog.Printf("inject skipped: no target window backend=%s elapsed=%.2fs", backendID, elapsed)
				cue.PlayError()
				continue
			}
			// The target window existed when F1 was pressed but may have been
			// closed during a slow transcription (e.g. the user moved from
			// patient A's record to patient B's). Fast-fail here with a
			// distinct diagnostic instead of letting RestoreForeground discover
			// the dead HWND via a thread-ID-0 error. RestoreForeground still
			// guards the same case, so this is a clarity/speed improvement, not
			// the sole line of defense.
			if !inject.IsWindow(res.targetHwnd) {
				fmt.Fprintf(os.Stderr, "inject target window gone, skipping (%.2fs)\n", res.elapsed.Seconds())
				daemonlog.Printf("inject skipped: target window gone backend=%s elapsed=%.2fs", backendID, elapsed)
				cue.PlayError()
				continue
			}
			inputMu.Lock()
			restoreOK, injectErr := inject.RestoreForeground(res.targetHwnd)
			if injectErr == nil && restoreOK {
				injectErr = inject.TypeAuto(text)
			}
			inputMu.Unlock()
			if injectErr != nil || !restoreOK {
				if injectErr != nil && restoreOK {
					fmt.Fprintf(os.Stderr, "inject: %v\n", injectErr)
				} else {
					fmt.Fprintf(os.Stderr, "inject restore foreground failed (ok=%v): %v\n", restoreOK, injectErr)
				}
				daemonlog.Printf("inject error backend=%s elapsed=%.2fs err=%v", backendID, elapsed, injectErr)
				cue.PlayError()
				continue
			}
			fmt.Fprintf(os.Stderr, "injected %q (%.2fs)\n", text, res.elapsed.Seconds())
			daemonlog.Printf("injected backend=%s elapsed=%.2fs chars=%d", backendID, elapsed, len([]rune(text)))
		case da := <-dictAdds:
			// Only this goroutine touches d, so Save+Reload need no lock.
			// dict.Save persists the rule even when d is nil (corrections
			// disabled at startup); reload the running dict only if there
			// is one.
			saved, err := dict.Save(da.wrong, da.correct)
			if err != nil {
				fmt.Fprintf(os.Stderr, "dict save: %v\n", err)
			}
			switch {
			case saved && d != nil:
				if rerr := d.Reload(); rerr != nil {
					fmt.Fprintf(os.Stderr, "dict reload: %v\n", rerr)
				} else {
					fmt.Fprintf(os.Stderr, "dict rule saved: %q = %q\n", da.wrong, da.correct)
				}
			case saved:
				fmt.Fprintf(os.Stderr, "dict rule saved (corrections disabled until restart): %q = %q\n", da.wrong, da.correct)
			}
		}
	}
}

// checkForUpdate queries GitHub for the latest release and reports the
// result as a tray balloon. It never downloads or installs — upgrades go
// through re-running `prata.exe --install` from the new copy (the single
// tested path), which also keeps Prata clear of the download-and-execute
// behaviour that AV/EDR products flag. Runs on its own goroutine so the
// network call never blocks the tray UI thread.
func checkForUpdate(t *tray.Tray) {
	res, err := update.Check(version)
	if err != nil {
		fmt.Fprintf(os.Stderr, "update check: %v\n", err)
		t.Notify("Prata", "Kunde inte söka efter uppdatering. Försök igen senare.")
		return
	}
	fmt.Fprintf(os.Stderr, "update check: current=%s latest=%s newer=%v\n", res.Current, res.Latest, res.Newer)
	switch {
	case res.Newer:
		t.Notify("Prata", fmt.Sprintf("Ny version %s finns (du kör %s). Uppdatera genom att köra om Prata-installationen från USB-minnet.", res.Latest, res.Current))
	case !res.Comparable:
		t.Notify("Prata", fmt.Sprintf("Senaste version är %s. Den här kopian är en lokal build (%s).", res.Latest, res.Current))
	default:
		t.Notify("Prata", fmt.Sprintf("Prata är uppdaterad (senaste är %s).", res.Latest))
	}
}

// preview returns the first n runes of s for log output, appending an
// ellipsis when truncated. Rune-based so Swedish characters (å, ä, ö) are
// never split mid-byte in the log.
func preview(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "..."
}

// f8Worker runs the F8 quick-fix flow off the listener goroutine: grab the
// foreground selection, let the user correct it in a popup, hand the rule
// to the processor goroutine for save+reload, restore focus to the source
// window, and paste the corrected text back over the selection. It always
// clears the single-flight guard on return.
func f8Worker(sourceHwnd uintptr, dictAdds chan<- dictAdd) {
	defer f8Busy.Store(false)

	inputMu.Lock()
	defer inputMu.Unlock()

	sel, ok, err := inject.CopySelection()
	if err != nil {
		fmt.Fprintf(os.Stderr, "f8 copy selection: %v\n", err)
		cue.PlayError()
		return
	}
	if !ok {
		cue.PlayError()
		return // nothing selected; clipboard already restored by CopySelection
	}

	leading, core, trailing := splitEnvelope(sel)
	if core == "" {
		cue.PlayError()
		return // selection was empty or all whitespace
	}

	edited, ok, err := popup.Prompt(core)
	if err != nil {
		fmt.Fprintf(os.Stderr, "f8 popup: %v\n", err)
		cue.PlayError()
		return
	}
	if !ok {
		return // Esc / clicked away: no save, no paste-back
	}
	edited = strings.TrimSpace(edited)
	if edited == "" || edited == core {
		// No-op: never delete the selected word, and skip re-injecting
		// unchanged text. This guard gates PASTE-BACK and is NOT redundant
		// with dict.Save's own identity filter — do not remove it.
		return
	}

	// Hand the rule to the processor goroutine (it owns d). This is sent
	// before the foreground gate so the rule persists even if paste-back is
	// aborted. If the processor is overloaded, abort paste-back too; showing
	// corrected text while silently losing the rule is worse than asking the
	// user to retry.
	select {
	case dictAdds <- dictAdd{wrong: core, correct: edited}:
	case <-time.After(500 * time.Millisecond):
		fmt.Fprintln(os.Stderr, "dict rule queue timeout, aborting paste-back")
		cue.PlayError()
		return
	}

	// Foreground gate: only paste once the source window is confirmed
	// foreground again, so a correction never lands in the wrong window.
	ok, err = inject.RestoreForeground(sourceHwnd)
	if err != nil || !ok {
		fmt.Fprintf(os.Stderr, "f8 restore foreground failed (ok=%v): %v\n", ok, err)
		cue.PlayError()
		return
	}

	// Paste-back over the still-selected word. No trailing newline (unlike
	// the dictation path) — this replaces an inline selection.
	if err := inject.TypeAuto(leading + edited + trailing); err != nil {
		fmt.Fprintf(os.Stderr, "f8 paste-back: %v\n", err)
		cue.PlayError()
	}
}

// splitEnvelope splits s into its leading whitespace run, the trimmed core,
// and its trailing whitespace run, so the F8 flow can show/save the core
// while reapplying the exact surrounding whitespace on paste-back. Splits
// are byte-offset-exact and rune-aware via unicode.IsSpace.
func splitEnvelope(s string) (leading, core, trailing string) {
	rest := strings.TrimLeftFunc(s, unicode.IsSpace)
	leading = s[:len(s)-len(rest)]
	core = strings.TrimRightFunc(rest, unicode.IsSpace)
	trailing = rest[len(core):]
	return leading, core, trailing
}
