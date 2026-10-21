#!/usr/bin/env bash
# DeskClaw 1줄 설치 (macOS) — Claude Code + DureClaw 버스 + deskclaw 를 한 번에.
#
#   curl -fsSL https://raw.githubusercontent.com/DureClaw/deskclaw/main/setup.sh | bash
#
# 이 맥 한 대로 시작(기본): DureClaw 플러그인 설치 → 버스(없으면 설치·백그라운드 실행)
#   → Work Key → DeskClaw.app 설치·설정 → 버스 접속 확인.
# 이미 있는 버스에 이 맥을 손(노드)으로 붙이기:
#   curl -fsSL …/setup.sh | bash -s -- --bus 100.64.0.10:4000 --token <OAH_SECRET>
#
# 옵션: --bus HOST:PORT  --token SECRET  --name AGENT_NAME  --work-key WK-…
#       --no-plugin (Claude Code 플러그인 건너뛰기)
# 남는 수동 단계는 macOS 권한(손쉬운 사용·화면 기록) 스위치 하나 — 앱이 처음 실행될 때 안내한다.
set -euo pipefail

REPO="DureClaw/deskclaw"
APP_ZIP_URL="${DESK_APP_ZIP_URL:-https://github.com/$REPO/releases/latest/download/DeskClaw-macos.zip}"
SERVER_URL="https://dureclaw.baryon.ai/server"
CONFIG_DIR="$HOME/Library/Application Support/deskclaw"
CONFIG="$CONFIG_DIR/config.json"

BUS="" TOKEN="" NAME="" WK="" PLUGIN=1
while [ $# -gt 0 ]; do
  case "$1" in
    --bus) BUS="$2"; shift 2 ;;
    --token) TOKEN="$2"; shift 2 ;;
    --name) NAME="$2"; shift 2 ;;
    --work-key) WK="$2"; shift 2 ;;
    --no-plugin) PLUGIN=0; shift ;;
    *) echo "알 수 없는 옵션: $1" >&2; exit 2 ;;
  esac
done

step() { printf '\n\033[1;38;5;208m▶ %s\033[0m\n' "$*"; }
ok()   { printf '  \033[32m✓\033[0m %s\n' "$*"; }
warn() { printf '  \033[33m!\033[0m %s\n' "$*"; }
die()  { printf '\n\033[31m✗ %s\033[0m\n' "$*" >&2; exit 1; }

[ "$(uname -s)" = "Darwin" ] || die "이 설치는 macOS 전용입니다. Windows/Linux는 install.sh(기본 기능)를 쓰세요."
TMP="$(mktemp -d)"; trap 'rm -rf "$TMP"' EXIT

# ── 1. Claude Code ← DureClaw 플러그인 ─────────────────────────────────────────
step "1/5 Claude Code에 DureClaw 플러그인"
if [ "$PLUGIN" = 0 ]; then
  ok "건너뜀 (--no-plugin)"
elif ! command -v claude >/dev/null 2>&1; then
  warn "Claude Code가 없습니다 — 나중에 설치 후: /plugin marketplace add DureClaw/dureclaw"
elif claude plugin list 2>/dev/null | grep -q 'dureclaw@'; then
  ok "이미 설치됨"
else
  claude plugin marketplace add DureClaw/dureclaw >/dev/null 2>&1 || true
  if claude plugin install dureclaw@dureclaw >/dev/null 2>&1; then
    ok "설치됨 — 열려 있는 Claude Code에서는 /reload-plugins 한 번"
  else
    warn "자동 설치 실패 — Claude Code에서 /plugin install dureclaw@dureclaw"
  fi
fi

# ── 2. DureClaw 버스 ─────────────────────────────────────────────────────────
step "2/5 DureClaw 버스"
health() { curl -fsS -m 3 "http://$1/api/health" >/dev/null 2>&1; }
if [ -n "$BUS" ]; then
  health "$BUS" || die "버스 $BUS 에 연결할 수 없습니다 (주소·포트·네트워크 확인)"
  ok "기존 버스 사용: $BUS"
else
  BUS="127.0.0.1:4000"
  if health "$BUS"; then
    ok "이 맥에서 이미 실행 중: $BUS"
  else
    echo "  버스를 설치하고 백그라운드로 실행합니다 (1~2분)…"
    curl -fsSL "$SERVER_URL" -o "$TMP/server.sh" || die "버스 설치 스크립트를 받지 못했습니다"
    DAEMON=1 bash "$TMP/server.sh" >"$TMP/server.log" 2>&1 || { tail -20 "$TMP/server.log"; die "버스 설치 실패"; }
    for _ in $(seq 1 60); do health "$BUS" && break; sleep 1; done
    health "$BUS" || die "버스가 시작되지 않았습니다 (~/.oah-server 확인)"
    ok "설치·실행됨: $BUS"
  fi
  if [ -z "$TOKEN" ]; then
    SECRET_FILE="${OAH_DATA_DIR:-$HOME/.oah-server/data}/server.secret"
    [ -r "$SECRET_FILE" ] || die "버스 토큰을 찾지 못했습니다 ($SECRET_FILE) — --token 으로 지정하세요"
    TOKEN="$(tr -d '[:space:]' < "$SECRET_FILE")"
  fi
fi
[ -n "$TOKEN" ] || die "버스 토큰이 필요합니다: --token <OAH_SECRET>"
AUTH="Authorization: Bearer $TOKEN"
curl -fsS -m 5 -H "$AUTH" "http://$BUS/api/presence" >/dev/null 2>&1 || die "버스 토큰이 맞지 않습니다 (401)"
ok "토큰 확인"

# ── 3. Work Key ─────────────────────────────────────────────────────────────
step "3/5 Work Key"
wk_of() { sed -n 's/.*"work_key" *: *"\([^"]*\)".*/\1/p' | head -1; }
if [ -z "$WK" ]; then
  WK="$(curl -fsS -m 5 -H "$AUTH" "http://$BUS/api/work-keys/latest" 2>/dev/null | wk_of || true)"
fi
if [ -z "$WK" ]; then
  WK="$(curl -fsS -m 5 -X POST -H "$AUTH" -H 'Content-Type: application/json' -d '{"goal":"deskclaw"}' \
        "http://$BUS/api/work-keys" | wk_of)"
fi
[ -n "$WK" ] || die "Work Key를 만들지 못했습니다"
ok "$WK"

# ── 4. DeskClaw.app ─────────────────────────────────────────────────────────
step "4/5 DeskClaw.app"
if curl -fsSL -m 120 "$APP_ZIP_URL" -o "$TMP/app.zip" 2>/dev/null && ditto -x -k "$TMP/app.zip" "$TMP/app" 2>/dev/null \
   && [ -d "$TMP/app/DeskClaw.app" ]; then
  ok "릴리스에서 받음"
  SRC_APP="$TMP/app/DeskClaw.app"
else
  warn "릴리스에 앱이 없어 소스에서 빌드합니다 (Go · Xcode Command Line Tools 필요)"
  command -v go >/dev/null || die "Go가 필요합니다: brew install go"
  command -v swiftc >/dev/null || die "Xcode Command Line Tools가 필요합니다: xcode-select --install"
  git clone -q --depth 1 "https://github.com/$REPO.git" "$TMP/src"
  make -C "$TMP/src" app >"$TMP/build.log" 2>&1 || { tail -20 "$TMP/build.log"; die "빌드 실패"; }
  SRC_APP="$TMP/src/dist/DeskClaw.app"
fi
DEST="/Applications"
[ -w "$DEST" ] || { DEST="$HOME/Applications"; mkdir -p "$DEST"; }
osascript -e 'quit app "DeskClaw"' >/dev/null 2>&1 || true
pkill -f 'DeskClaw.app/Contents/MacOS/' 2>/dev/null || true
sleep 1
rm -rf "$DEST/DeskClaw.app"
ditto "$SRC_APP" "$DEST/DeskClaw.app"
ok "$DEST/DeskClaw.app"

# ── 5. 설정 → 실행 → 접속 확인 ─────────────────────────────────────────────────
step "5/5 설정하고 연결"
[ -n "$NAME" ] || NAME="deskclaw@$(scutil --get LocalHostName 2>/dev/null || hostname -s)"
mkdir -p "$CONFIG_DIR"
jstr() { printf '"%s"' "$(printf '%s' "$1" | sed 's/\\/\\\\/g; s/"/\\"/g')"; }
if [ -s "$CONFIG" ] && [ "$(tr -d '[:space:]' < "$CONFIG")" != "{}" ]; then
  for kv in "STATE_SERVER=$BUS" "OAH_SECRET=$TOKEN" "WORK_KEY=$WK" "AGENT_NAME=$NAME"; do
    plutil -replace "${kv%%=*}" -string "${kv#*=}" "$CONFIG"   # stays JSON; other settings kept
  done
else
  printf '{"STATE_SERVER":%s,"OAH_SECRET":%s,"WORK_KEY":%s,"AGENT_NAME":%s}\n' \
    "$(jstr "$BUS")" "$(jstr "$TOKEN")" "$(jstr "$WK")" "$(jstr "$NAME")" > "$CONFIG"
fi
chmod 600 "$CONFIG"
ok "설정 저장 (토큰 포함, 본인만 읽기): $CONFIG"

open "$DEST/DeskClaw.app"
printf '  버스 접속 대기'
JOINED=0
for _ in $(seq 1 40); do
  if curl -fsS -m 3 -H "$AUTH" "http://$BUS/api/presence" 2>/dev/null | grep -q "\"$NAME\""; then JOINED=1; break; fi
  printf '.'; sleep 1
done
echo
[ "$JOINED" = 1 ] && ok "$NAME 이(가) 버스에 접속했습니다" || warn "아직 접속이 안 보입니다 — 메뉴바 발톱 아이콘 → 로그 보기"

cat <<EOF

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
 ✅ 설치 완료 — 남은 일은 하나입니다

 ① 방금 뜬 권한 창에서 '손쉬운 사용'과 '화면 기록'에 DeskClaw를 켜세요.
    (창을 놓쳤다면: 메뉴바 발톱 아이콘 → 권한 확인)

 그다음 Claude Code에서 이렇게 말해 보세요:
    "두레팀 상태 알려줘"
    "$NAME 에서 실행 중인 앱 목록 보여줘"
    "카톡에서 답장해야 할 방만 골라줘"

 버스  : http://$BUS   (대시보드)
 노드  : $NAME   ·   Work Key: $WK
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
EOF
