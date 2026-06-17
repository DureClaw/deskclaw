# deskclaw

> The **DureClaw desktop GUI node** — lends a **Windows / macOS desktop** to a
> [DureClaw](https://github.com/DureClaw/dureclaw) fleet: **eyes** (screenshot) +
> **hands** (click · type · key · scroll). **The master brings the brain**
> (Claude *computer-use* vision decides *what* to do); deskclaw carries it out.

deskclaw is the **desktop counterpart of [webclaw](https://github.com/DureClaw/webclaw)**
(which lends the browser). Where [edgeclaw](https://github.com/DureClaw/edgeclaw) is a
*headless* OS node (shell/sensor/GPIO), deskclaw is the **GUI** node — it sees the screen and
operates native apps for the fleet.

```
edgeclaw = OS headless hands (shell/sensor/GPIO)   — every OS/CPU, static binary
webclaw  = browser hands (fetch/DOM)               — Chrome extension
deskclaw = desktop GUI eyes+hands (screenshot·click·type) — Windows/macOS native
```

## Why this isn't edgeclaw

GUI control needs per-OS native APIs (Windows UIAutomation/SendInput, macOS
Accessibility/CGEvent), OS permissions (Screen Recording, Accessibility), and a vision model
to *understand* the screen — that breaks edgeclaw's "tiny, keyless, no-model" ethos. So it's a
separate family member. Consistency is kept where it matters: **same bus wire protocol**
(Phoenix Channel `vsn=2.0.0`, 5-tuple frames) and the **keyless master-brings-the-brain**
delegation. The master's vision grounds a screenshot → an action; deskclaw executes it.

## Pure-Go, no CGo — built-in OS tooling

No native-linked libraries. deskclaw shells out to what each OS already ships:

| | eyes (screenshot) | hands (pointer) | hands (keyboard) |
|---|---|---|---|
| **macOS** | `screencapture` ✅ built-in | `cliclick` *(optional: `brew install cliclick`)* | `osascript` System Events ✅ built-in |
| **Windows** | PowerShell `System.Drawing` ✅ | user32 P/Invoke ✅ built-in | `SendKeys` ✅ built-in |

Same one-file install story as edgeclaw; Windows needs **zero** extra installs.

## Task verbs (over the bus)

A master fans out a `task.assign` with one of these instructions:

| Verb | Action |
|------|--------|
| `[SCREENSHOT]` | capture the screen → returns saved path + `WxH` (add `b64` or `DESK_SHOT_B64=1` to inline a data URL) |
| `[LAUNCH] <app\|path>` | locate & launch a program (macOS `open -a` / Windows `Start-Process`) |
| `[CLICK] x y` · `[DOUBLECLICK] x y` · `[RIGHTCLICK] x y` | mouse click at coordinates |
| `[MOVE] x y` | move the pointer |
| `[TYPE] <text>` | type text into the focused field |
| `[KEY] <combo>` | press a key/combo — `cmd+c`, `ctrl+shift+t`, `enter`, `tab`, `esc`, arrows… |
| `[SCROLL] x y amount up\|down` | scroll wheel (Windows; macOS MVP: use `[KEY] pagedown`) |
| `[RECORD] <name>` … `[ENDREC]` | **record** the actions in between into a named macro |
| `[RUN] <name>` · `[MACROS]` | **replay** a macro deterministically (no LLM) · list macros |
| *(anything else)* | delegate to the **master brain** (`BRAIN_URL`/brain/exec), keyless |

The intended loop: master sends `[SCREENSHOT]` → grounds the image with vision → sends
`[LAUNCH]/[CLICK]/[TYPE]/[KEY]` → repeat. (Screenshot upload to the brain is a phase-2 add;
today the image is returned as a path, or inline base64 for small screens.)

## RPA: teach once with the LLM, then replay with no LLM

deskclaw's point isn't to call the LLM on every click forever — it's to **learn a procedure
once and crystallize it into a fast deterministic macro**. Same "LLM as compiler" loop as
DureClaw skill crystallization, applied to the desktop:

```
[RECORD] open-and-export          ● start recording
[LAUNCH] MyApp                    ← master (vision) drives these,
[CLICK] 412 88                       one grounded step at a time
[CLICK] 300 510                      (locate program · run · click the right menu · …)
[TYPE]  monthly-report
[KEY]   cmd+s
[ENDREC]                          ■ saved → ~/…/deskclaw/macros/open-and-export.json

# next time — no LLM, no vision, just fast replay:
[RUN] open-and-export             ▶ replays every step deterministically
```

Verified end-to-end on macOS: record `[LAUNCH]` → save → `[RUN]` re-launches with **zero LLM
calls**. Macros are plain JSON under `DESK_MACRO_DIR` (default OS config dir), so they're
inspectable, editable, and shareable across the fleet.

> MVP records coordinate-based steps; **element-targeting via the accessibility tree**
> (UIAutomation / AX) is the planned robustness upgrade so macros survive window moves.

## Install / run

```bash
# build from source (no toolchain tricks; CGo not required)
go build -o deskclaw .        # or: make build

STATE_SERVER=<bus-host:4000> OAH_SECRET=<token> WORK_KEY=<WK> \
  AGENT_NAME=deskclaw@$(hostname) \
  BRAIN_URL=http://<master>:4111 BRAIN_TOKEN=<tok> \
  ./deskclaw
```

**Permissions (macOS):** first run prompts for **Screen Recording** (screenshots) and
**Accessibility** (keyboard/pointer). Grant them to the terminal/binary running deskclaw.

## Configuration (env)

| Env | Meaning |
|-----|---------|
| `STATE_SERVER` / `OAH_SECRET` / `WORK_KEY` | bus `host:port` · bearer token · session key |
| `AGENT_NAME` / `AGENT_ROLE` / `CAPABILITIES` | fleet identity |
| `BRAIN_URL` / `BRAIN_TOKEN` | master brain endpoint — keyless LLM for natural-language tasks |
| `DESK_SHOT_DIR` | where screenshots are saved (default: temp dir) |
| `DESK_SHOT_B64` | inline screenshots as base64 data URLs in the result |
| `DESK_MACRO_DIR` | where recorded RPA macros are stored (default: OS config dir `…/deskclaw/macros`) |

## Status

MVP. Verified on macOS against a live DureClaw bus: node joins (presence), `[SCREENSHOT]`
captures the real screen (e.g. 3024×1964), and the **RPA loop** records `[LAUNCH]` → saves a
macro → `[RUN]` replays it with **zero LLM calls**. Pointer/keyboard verbs implemented for
macOS (osascript + optional cliclick) and Windows (PowerShell, fully built-in); cross-builds
clean for darwin + windows (amd64/arm64). Linux is a join-only stub (X11/Wayland backend TBD).

> Roadmap: **accessibility-tree targeting** (UIAutomation / AX) so macros hit *elements* not
> pixels and survive window moves · screenshot→brain upload loop · macro parameters/variables ·
> `[WAIT]`/`[FINDTEXT]` guards · Linux backend.

---

_Family: [edgeclaw](https://github.com/DureClaw/edgeclaw) (OS node) ·
[webclaw](https://github.com/DureClaw/webclaw) (browser node) · **deskclaw** (desktop GUI node).
The master brings the brain; each claw brings different hands._
