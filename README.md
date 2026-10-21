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

## 한눈에 보기 — 무엇을, 얼마나 빠르게

**한 줄 요약:** deskclaw는 내 컴퓨터의 앱을 AI가 대신 눌러 주는 "손"입니다. 사람이 마우스로
하던 일을 AI가 원격에서 시키면, deskclaw가 화면 속 버튼과 입력칸을 직접 찾아 누릅니다.

> 🚀 **처음이라면 — 맥에서 1줄:**
> ```bash
> curl -fsSL https://raw.githubusercontent.com/DureClaw/deskclaw/main/setup.sh | bash
> ```
> Claude Code 플러그인 · DureClaw 버스 · DeskClaw.app 설치와 연결까지 자동. 남는 건 macOS 권한 켜기 하나.
> 자세한 안내: [빠른 시작 — Claude Code + DureClaw + deskclaw](docs/QUICKSTART.ko.md)

### 왜 빠른가 — 화면을 "보지" 않고 "읽는다"

보통의 컴퓨터 조작 AI는 **스크린샷을 찍고 → 그림을 해석하고 → 좌표를 클릭**합니다. 한 동작에
수십 초가 걸리고, 창이 조금만 움직여도 엉뚱한 곳을 누릅니다.

deskclaw는 macOS 접근성 기능으로 화면을 **글자처럼 읽습니다.** "저장 버튼", "메시지 입력칸"을
이름으로 찾아서 바로 누르기 때문에 그림 해석이 필요 없습니다.

| 하는 일 (실측, macOS · 카카오톡) | 스크린샷 방식 | deskclaw |
|---|---|---|
| 채팅 목록 읽기 | 54초 (20개) | **약 2.5초 (60개)** |
| 채팅방 열고 최근 메시지 읽기 | 59초 + 68초 | **1.4초** (이미 열린 방은 0.17초) |
| 메시지 보내기 | 약 15초 | **0.3초** (전송 확인 포함, kax 기준) |
| 채팅방 전체 기록 내보내기 | — | **약 12초** (993개, 9개월치 CSV) |
| 버튼 하나 누르기 (계산기) | — | **0.05~0.08초** |

> 화면에 "AI가 이렇게 움직이고 있다"를 보여주는 가상 커서를 켜면 한 동작당 약 0.7초가 더
> 듭니다. 사람이 지켜볼 때는 켜고, 밤새 혼자 돌릴 때는 끄면 됩니다(`DESK_VISUAL=0`).

### 할 수 있는 일

- **어떤 앱이든 조작** — 버튼 누르기, 입력칸 채우기, 메뉴 고르기, 화면 글자 읽기 (`[AX]`)
- **한 번 가르치면 반복은 공짜** — 처음엔 AI가 화면을 보며 단계별로 조작하고, 그 과정을
  매크로로 녹화해 두면 다음부터는 AI 호출 없이 몇 초 만에 재생 (`[RECORD]` → `[RUN]`)
- **카카오톡 전용 기능 내장** — 안 읽은 방 찾기, 답장 필요한 방 분류, 읽기, 보내기,
  전체 대화 내보내기 (`[KAKAO]`)
- **사람이 안심하고 맡기게** — 조작 중 주황 테두리·배너·가상 커서로 표시, 메시지 전송 같은
  실제 동작은 화면에 "허용/거부"를 띄워 사람이 확인, 작업 중 연 창은 끝나면 자동으로 닫음

### 가상 사례

아래는 이해를 돕기 위한 **가상의 상황**입니다. 시간은 위 실측값으로 계산한 **추정치**입니다.

**① 아침 카톡 정리 — 자영업자 A씨**
단톡방과 고객 방이 60개 넘게 쌓여 있습니다. 출근길에 AI에게 "답장할 것만 골라 줘"라고
시키면, deskclaw가 목록을 읽고 분류합니다(`[KAKAO] triage`, 약 3초). AI가 답장이 필요한 방
5개의 초안을 쓰고, A씨는 데스크톱에 뜬 "허용" 버튼만 누릅니다.
→ 사람이 직접 하면 20~30분, 여기서는 **1분 안팎 + 승인 클릭 5번**.

**② 고객 단톡방 월간 보관 — 컨설팅 회사 B팀**
매달 말 고객사 단톡방 10개의 대화를 내려받아 보관해야 합니다. `[KAKAO] export`를 방마다
실행하면 카카오톡의 공식 내보내기 기능으로 빠짐없이 CSV가 저장됩니다.
→ 방당 약 12초, 10개 방 **약 2분**. 열렸던 설정 창은 자동으로 닫힙니다.

**③ 사내 구형 프로그램 월말 보고서 — 경리 담당 C씨**
API도 없는 오래된 회계 프로그램에서 매달 같은 메뉴를 눌러 보고서를 뽑습니다. 첫 달에는 AI가
화면을 보며 한 단계씩 조작하고(몇 분), 그 과정을 매크로로 녹화합니다. 다음 달부터는
`[RUN] 월말보고서` 한 줄로 **수 초 안에** 같은 일을 반복합니다. AI 비용도 들지 않습니다.

**④ 엑셀 데이터를 데스크톱 프로그램에 입력 — 영업지원 D씨**
거래처 100곳 × 입력칸 8개를 데스크톱 전용 프로그램에 옮겨 적어야 합니다. 입력칸을 이름으로
찾아 채우므로(`[AX] set`, 입력값을 다시 읽어 확인) 사람이 하면 반나절 걸리는 일이
→ 가상 커서를 끄면 **2분 안팎**, 켜고 지켜보며 하면 **10분 안팎** (추정). 단, 그 프로그램의
입력칸이 접근성 정보(이름)를 제공해야 합니다. 제공하지 않으면 좌표 클릭으로 대신해서 더 느립니다.

**⑤ 사무실 맥 여러 대 야간 작업 — IT 담당 E씨**
맥 5대에 DeskClaw를 깔아 두면 한 곳의 AI 마스터가 밤사이 각 PC에 일을 나눠 줍니다.
각 PC의 메뉴바에서 무엇을 했는지 최근 작업 목록과 알림으로 확인할 수 있습니다.

> 현재 범위: 이름으로 찾아 누르는 요소 조작(`[AX]`), 카카오톡 기능, 메뉴바 앱, 가상 커서는
> **macOS 전용**입니다. Windows는 스크린샷·좌표 클릭·키 입력만 지원합니다.
>
> 주의: 카카오톡 이용약관은 자동화 프로그램 사용을 제한합니다. 대량 발송이나 짧은 주기의
> 반복 실행은 하지 마세요.

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

**macOS, everything in one line** (Claude Code plugin + DureClaw bus + DeskClaw.app, configured
and connected; only the macOS permission toggle is left to you):

```sh
curl -fsSL https://raw.githubusercontent.com/DureClaw/deskclaw/main/setup.sh | bash
# join an existing bus instead:  … | bash -s -- --bus HOST:4000 --token <OAH_SECRET> --no-plugin
```

**CLI node only** — prebuilt — one line, auto-detects your OS/CPU (macOS · Windows · Linux-stub):

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
