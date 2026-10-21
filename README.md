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
On macOS, element-level control (`[AX]`, app adapters, the on-screen indicator) uses a small
Swift helper whose source is **embedded in the binary** and compiled once with `swiftc`
(Xcode Command Line Tools) into the user cache dir.

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
| `[MARK]` · `[RESTORE]` | push a window snapshot · close windows opened since (see *Window stack*) |
| `[AX] apps\|tree\|find\|text\|press\|set …` | **any app, by element** (macOS Accessibility) — see below |
| `[KAKAO] rooms\|unread\|triage\|read\|send …` · `[APPS]` | **built-in app adapter** (KakaoTalk) · list adapters |
| `[<VERB>] …` · `[PLUGINS]` | run a **plugin** (an external CLI registered by manifest) · list plugins |
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

> Coordinate steps (`[CLICK] x y`) break when a window moves; on macOS record with
> `[AX] press/set` instead — element-targeted steps survive moves (see below).

## Any app, by element — `[AX]` (macOS)

Pixel clicks break when a window moves. `[AX]` drives **any** app through the macOS
Accessibility API by *element* — role, title, description, value — and reads the UI as
text, so the master often doesn't need a screenshot at all.

```
[AX] apps                                   running GUI apps
[AX] tree  Safari --depth 8 --max 80        element tree: path role "label"
[AX] find  Calculator "role=AXButton"       matches with path + screen frame
[AX] text  Notes                            readable text of the windows (or under a match)
[AX] press Calculator "role=AXButton 등호"   press button/menu/row (falls back to a real click)
[AX] set   Safari "role=AXTextField" "dureclaw"   fill a field, verified by read-back
```

`axhelper` (what `[AX]` and adapters run) also has `dump` (JSON lines), `locate` (target +
candidate frames), `pick` (select a list row + Return, raise its window), `key`, `windows`,
`close` (a window's close button), `rowcount` and `scroll <0..1>`. Open pop-up/context menus
are searched too; `--menus` adds the menu bar. Quote values with spaces:
`"role=AXMenuItem title=\"채팅방 설정\""`.

`<app>` is a name (localized or bundle file name — `Calculator` finds 계산기), bundle id or
pid. A query is words the label must contain, plus `role=` `title=` `desc=` `value=`
(exact) / `title~` `desc~` `value~` (contains), `#2` (2nd match), `path=0.1.3` (from tree).
Options: `--depth` `--max` `--rows` `--tail` `--window <title>` `--menus` `--no-activate`.

`[AX] press` / `set` are **recorded into RPA macros** — element-targeted, so replays survive
window moves (verified: record 7 + 5 = on Calculator → `[RUN]` → 12, no LLM).
The Swift helper is embedded in the binary and built once with `swiftc` (Xcode Command
Line Tools) into the user cache dir. Windows UIAutomation is the next backend.

## Built-in app adapters — `[KAKAO]`, …

`[AX]` is the universal layer; an **app adapter** is deskclaw knowing *one* app well — which
window/table holds what, how to parse rows into rooms/messages, how to verify an action
really happened. Adapters are compiled in (`app_<name>.go` → `registerApp` in `init()`), so
they ship in the single binary and get help, the on-screen indicator and the confirm guard
for free. `[APPS]` lists them.

**KakaoTalk (macOS)** — ported from the `kax` + `kakao.py` tool:

```
[KAKAO] rooms -n 30              chat list (does not open rooms)
[KAKAO] unread -n 60             rooms with unread messages
[KAKAO] triage -n 60             unread + category / needs_reply / urgency (Jev, else rules)
[KAKAO] read "방 이름" -n 20      open the room → recent messages (clears the '1' for the sender)
[KAKAO] send "방 이름" "내용"      dry-run — prints what would be sent
[KAKAO] send "방 이름" "내용" --yes   real send, verified (input cleared + text in the latest rows)
[KAKAO] read "방 이름" --all     scroll to the top until no older messages load, then read all
[KAKAO] export "방 이름"          KakaoTalk's own export → full-history CSV (Date, User, Message)
```

`export` drives KakaoTalk's built-in 채팅방 설정 › 대화 내용 관리 › 텍스트 파일로 저장 and returns
the file path plus message count / date range — the complete history in ~12 s (verified: 993
messages, Jan → Oct, where scrolling stopped at the ~1,100 rows KakaoTalk keeps loaded).

Add `--json` for machine output. Measured: room open + read 1.4 s, re-read 0.17 s, list of
60 rooms ≈ 2.5 s. `triage` uses **Jev** (TypeSafe System One) when `TYPESAFE_API_KEY` is
set, otherwise rules. KakaoTalk only exposes windows while frontmost, so it comes to the
front during a command. Its terms restrict automation — no bulk sends or tight polling.

**Confirm guard:** an adapter's confirm flag (`--yes`) acts on the real world, so it's
refused unless the **node** opts in with `DESK_ALLOW_CONFIRM=KAKAO` — a remote master can't.

## Plugins — any external CLI as a verb

For tools you'd rather not compile in (an ERP export script, a CRM CLI…), drop a manifest
into `DESK_PLUGIN_DIR` (default `~/Library/Application Support/deskclaw/plugins` on macOS,
`%AppData%\deskclaw\plugins` on Windows) — no rebuild:

```json
{
  "verb": "REPORT",
  "description": "monthly ERP export",
  "command": ["python3", "/abs/path/erp_export.py"],
  "os": ["darwin", "windows"],
  "subcommands": ["list", "export"],
  "front_flags": ["--json"],
  "confirm_flag": "--yes",
  "allow_confirm": false,
  "timeout_sec": 120,
  "record": false,
  "overlay": true
}
```

`[REPORT] export "2026-10"` runs `python3 erp_export.py export 2026-10` (quotes group args;
`front_flags` are hoisted before the subcommand). A `confirm_flag` arg is refused unless the
node's manifest says `allow_confirm: true`. `record: true` lets steps join RPA macros;
`overlay: true` shows the on-screen indicator. `[PLUGINS]` reloads and lists. Minimal example:
[`plugins/say.json.example`](plugins/say.json.example).

## On-screen indicator + virtual cursor

Whenever deskclaw drives the GUI — pointer/keyboard verbs, `[AX]`, `[RUN]` replays, app
adapters — the person at the machine sees a **blinking orange border and a top banner**
("Claude Computer Use 실행 중 · deskclaw (<agent>) — 마우스·키보드를 건드리지 마세요"). It
never takes focus and lets clicks through. Steps from a master share one indicator that
lingers 3 s after the last step (no flicker), and it's dropped right before `[SCREENSHOT]`
so it never shows up in the image the master's vision reads. It exits by itself if deskclaw
dies or after 180 s. macOS today; `DESK_OVERLAY=0` turns it off, `--no-overlay` skips one task.

AX presses don't move the real pointer, so buttons would seem to react by themselves. The
overlay therefore **shows what the agent is doing**: before an `[AX] press/set` it outlines
the candidate elements it is choosing among (dashed blue), highlights the pick in orange
with a "🎯 label" tag, glides an orange **virtual cursor** tagged "Claude" onto it, then
shows a click ripple. `[CLICK] x y` glides the cursor too; `[TYPE]`/`[KEY]`/field fills show a
"⌨ …" caption. It costs ~0.7 s per step; `DESK_VISUAL=0` keeps the border but skips it.

## Window stack — leave the desktop as it was

Jobs open windows only as a means — a settings window, a save panel, a Finder window, a chat
room. deskclaw keeps a **stack of window snapshots**: before a job it pushes the set of
windows (by window number, across all Spaces, regular apps only); afterwards it pops it and
closes every window that appeared in between, frontmost first. Windows that were already
open — even if moved — are left alone.

- Automatic around app adapters (`[KAKAO] …`) and `[RUN]` macro replays.
- A master driving step by step brackets a job itself: `[MARK]` … `[RESTORE]` (nestable).
- Safety: if more than 6 "new" windows show up, it closes nothing and says so.
- `--keep-windows` on one task, or `DESK_WINSTACK=0`, keeps everything open.

## DeskClaw.app — menu-bar app (macOS)

`make app` builds **`dist/DeskClaw.app`** (universal arm64 + x86_64; `make install-app` copies
it to /Applications and opens it): the Swift menu-bar shell + the Go node + a prebuilt
axhelper (no `swiftc` needed on the target Mac), with its own icon.

- **Menu bar**: claw icon shows the state (solid = connected, faint = connecting, slashed =
  disconnected/paused, dot = working), recent tasks (✓ / ✗ / ▶), pause/resume, open log,
  open the config folder (macros, plugins), launch at login, permission check.
- **Settings window**: bus server, token, work key, agent name, brain URL/token, adapters
  always allowed to act, Jev key, on-screen indicator. Saved to
  `~/Library/Application Support/deskclaw/config.json` (mode 600) and applied by restarting the node.
- **Notifications**: bus disconnected, task failed (optionally every finished task).
- **Approve in place**: a confirm-flagged action (`[KAKAO] send … --yes`) pops an
  Allow/Deny dialog for the person at the machine (90 s, then deny) instead of needing
  `DESK_ALLOW_CONFIRM`. The overlay steps aside while the dialog is up.
- **Permissions** go to DeskClaw.app once. It's signed with your Apple Development identity
  when one is installed, so they survive rebuilds (ad-hoc otherwise; override `SIGN=`).

The shell runs the node with `DESK_UI=1` and talks JSON lines over stdin/stdout (`ui.go`).
`make icon` regenerates `app/DeskClaw.icns` from `app/icon.swift`.

## Install / run

Prebuilt — one line, auto-detects your OS/CPU (macOS · Windows · Linux-stub):

```sh
curl -fsSL https://github.com/DureClaw/deskclaw/releases/latest/download/install.sh | sh
```

Or grab a binary from **[Releases](https://github.com/DureClaw/deskclaw/releases/latest)**
(darwin/windows `amd64`·`arm64`, linux `amd64`; SHA256SUMS attached), or build from source:
`go build -o deskclaw .` (CGo not required) / `make build`.

```bash
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
| `DESK_PLUGIN_DIR` | plugin manifests (default: OS config dir `…/deskclaw/plugins`) |
| `DESK_AX_HELPER` | use a prebuilt axhelper binary instead of compiling the embedded one |
| `DESK_MAX_OUTPUT` | max chars of a task result sent back to the bus (default 4000) |
| `DESK_ALLOW_CONFIRM` | adapters allowed to act for real, e.g. `KAKAO` (enables `[KAKAO] send … --yes`) |
| `DESK_OVERLAY` | `0` disables the on-screen "computer use running" indicator |
| `DESK_VISUAL` | `0` keeps the indicator but skips the virtual cursor / candidate highlights |
| `DESK_WINSTACK` | `0` = don't close windows a job opened (window stack off) |
| `DESK_UI` | `1` = run under DeskClaw.app (JSON-line events + approvals on stdin/stdout) |
| `TYPESAFE_API_KEY` / `TYPESAFE_MODEL` | Jev for adapter judgments (e.g. `[KAKAO] triage`); rules otherwise |

## Status

MVP. Verified on macOS against a live DureClaw bus: node joins (presence), `[SCREENSHOT]`
captures the real screen (e.g. 3024×1964), and the **RPA loop** records `[LAUNCH]` → saves a
macro → `[RUN]` replays it with **zero LLM calls**. Pointer/keyboard verbs implemented for
macOS (osascript + optional cliclick) and Windows (PowerShell, fully built-in); cross-builds
clean for darwin + windows (amd64/arm64). Linux is a join-only stub (X11/Wayland backend TBD).

> Roadmap: **Windows UIAutomation** backend for `[AX]` · screenshot→brain upload loop · macro parameters/variables ·
> `[WAIT]`/`[FINDTEXT]` guards · Linux backend.

---

_Family: [edgeclaw](https://github.com/DureClaw/edgeclaw) (OS node) ·
[webclaw](https://github.com/DureClaw/webclaw) (browser node) · **deskclaw** (desktop GUI node).
The master brings the brain; each claw brings different hands._
