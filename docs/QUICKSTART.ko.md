# 빠른 시작 — Claude Code + DureClaw + deskclaw

Claude Code에게 말로 시키면, 내 맥의 앱을 deskclaw가 대신 조작하게 만드는 30분짜리 가이드입니다.

```
 나 ──말로 지시──▶ Claude Code (두뇌)
                     │  DureClaw 플러그인
                     ▼
              DureClaw 버스 (Phoenix 서버, :4000)
                     │
                     ▼
              deskclaw (손) ──▶ 카카오톡 · 계산기 · 사내 프로그램 …
```

- **Claude Code** — 무엇을 할지 판단하는 두뇌(마스터)
- **DureClaw** — 두뇌와 손을 잇는 메시지 버스
- **deskclaw** — 실제로 화면을 읽고 누르는 손

한 대의 맥에서 셋을 모두 돌려도 되고, 버스와 Claude Code는 내 맥에, deskclaw는 다른 맥에 둬도 됩니다.

---

## ⚡ 1줄 설치 (권장)

터미널에 한 줄이면 아래 1~4단계가 모두 끝납니다.

```bash
curl -fsSL https://raw.githubusercontent.com/DureClaw/deskclaw/main/setup.sh | bash
```

자동으로 하는 일:

1. Claude Code에 DureClaw 플러그인 설치 (이미 있으면 건너뜀)
2. DureClaw 버스 확인 — 없으면 설치해서 백그라운드로 실행
3. 버스 토큰 읽기 → Work Key 확보
4. DeskClaw.app 설치(`/Applications`) → 설정 자동 입력 → 실행 → 버스 접속 확인

**남는 일은 하나:** 뜨는 권한 창에서 **손쉬운 사용**과 **화면 기록**에 DeskClaw를 켜는 것.
(macOS 보안 정책상 이것만은 스크립트가 대신 켤 수 없습니다. 켜면 "권한 허용됨" 알림이 옵니다.)
그다음 바로 [5. 첫 작업](#5-첫-작업--말로-시키기)으로 가세요.

**다른 맥을 손으로 붙일 때** — 버스가 이미 다른 곳(예: 사무실 맥)에 있으면, 붙일 맥에서:

```bash
curl -fsSL https://raw.githubusercontent.com/DureClaw/deskclaw/main/setup.sh \
  | bash -s -- --bus 100.64.0.10:4000 --token <버스 토큰> --no-plugin
```

| 옵션 | 뜻 |
|---|---|
| `--bus HOST:PORT` | 기존 버스 주소 (없으면 이 맥에 버스를 띄움) |
| `--token SECRET` | 버스 토큰 (이 맥에 버스가 있으면 자동으로 읽음) |
| `--name 이름` | 노드 이름 (기본 `deskclaw@컴퓨터이름`) |
| `--work-key WK-…` | 쓸 Work Key (기본: 버스의 최근 키, 없으면 새로 만듦) |
| `--no-plugin` | Claude Code 플러그인 설치 건너뛰기 (손 역할만 하는 맥) |

---

## 단계별 설치 (직접 하고 싶을 때)

아래 1~4단계는 1줄 설치가 하는 일을 손으로 하는 방법입니다.

## 0. 준비물

| 항목 | 내용 |
|---|---|
| 맥 | macOS 13 이상 (요소 조작·카카오톡 기능·메뉴바 앱은 macOS 전용) |
| Claude Code | 설치·로그인 완료 |
| 빌드 도구 | 단계별로 소스에서 빌드할 때만: Go 1.24+, Xcode Command Line Tools |
| (여러 대일 때) | 모든 기기가 같은 네트워크 또는 [Tailscale](https://tailscale.com) |

> 1줄 설치는 GitHub 릴리스의 `DeskClaw-macos.zip`을 받습니다. 릴리스에 없으면 소스에서 빌드하므로
> 그때만 Go와 Xcode Command Line Tools가 필요합니다.

---

## 1. Claude Code에 DureClaw 플러그인 설치 (2분)

Claude Code 프롬프트에 차례로 입력합니다.

```
/plugin marketplace add DureClaw/dureclaw
/plugin install dureclaw@dureclaw
/reload-plugins
/dureteam-status
```

마지막 명령에서 "팀 없음 / 서버 미연결"이 나오면 정상입니다. 아직 버스를 안 띄웠기 때문입니다.

## 2. DureClaw 버스 띄우기 (5분)

가장 쉬운 방법은 Claude Code에게 맡기는 것입니다.

```
/setup-team
```

또는 터미널에서 직접:

```bash
bash <(curl -fsSL https://dureclaw.baryon.ai/server)
```

끝나면 두 가지를 메모합니다.

1. **버스 주소** — 예: `100.64.0.10:4000` (한 대에서만 쓰면 `127.0.0.1:4000`)
2. **버스 토큰(OAH_SECRET)** — 서버를 처음 띄울 때 만들어져 서버 데이터 폴더의 `server.secret` 파일에
   저장됩니다. 서버를 `OAH_SECRET=…` 환경변수로 띄웠다면 그 값입니다.

그다음 작업 묶음 키(**Work Key**)를 하나 만듭니다. deskclaw는 이 키의 채널에서 일을 받습니다.

```bash
curl -s -X POST http://<버스 주소>/api/work-keys \
  -H "Authorization: Bearer <버스 토큰>" -H "Content-Type: application/json" -d '{}'
# → {"work_key":"WK-xxxxxxxx"}
```

> 버스는 작업에 Work Key가 없으면 **가장 최근 키**로 보냅니다. 그래서 키를 하나만 쓰면 따로 신경 쓸 일이 없습니다.

## 3. deskclaw 설치 — DeskClaw.app (5분)

deskclaw를 돌릴 맥에서:

```bash
git clone https://github.com/DureClaw/deskclaw.git
cd deskclaw
make install-app        # 빌드 → /Applications/DeskClaw.app 복사 → 실행
```

처음 실행하면 **설정 창**이 뜹니다.

| 칸 | 넣을 값 |
|---|---|
| 버스 서버 | 2단계의 버스 주소 (`100.64.0.10:4000`) |
| 버스 토큰 | 2단계의 OAH_SECRET |
| Work Key | 2단계에서 만든 `WK-…` |
| 에이전트 이름 | 이 맥을 부를 이름 (예: `deskclaw@내맥`) |
| 항상 허용 어댑터 | 비워 두세요 — 메시지 전송 같은 실제 동작을 매번 화면에서 물어봅니다 |
| Jev 키 | 선택 — 카카오톡 분류를 더 정확하게 (없으면 규칙 기반) |

**저장하고 연결**을 누른 뒤, 메뉴바의 발톱 아이콘 → **권한 확인**을 눌러
**손쉬운 사용**과 **화면 기록**에서 DeskClaw를 켭니다. 켠 뒤 노드가 자동으로 다시 시작됩니다.

> 터미널에서 바로 돌리고 싶다면(앱 없이):
> ```bash
> go build -o deskclaw . && STATE_SERVER=<버스 주소> OAH_SECRET=<토큰> WORK_KEY=<WK-…> \
>   AGENT_NAME=deskclaw@내맥 ./deskclaw
> ```
> 이 경우 권한은 그 터미널 앱에 줘야 하고, 메뉴바·알림·승인 창은 없습니다.

## 4. 연결 확인 (1분)

- 메뉴바 발톱 아이콘이 **진하게** 바뀌면 연결된 것입니다. (흐림 = 연결 중, 사선 = 끊김)
- Claude Code에서:

```
두레팀 상태 알려줘
```

온라인 목록에 `deskclaw@내맥`이 보이면 준비 완료입니다.

## 5. 첫 작업 — 말로 시키기

Claude Code에 자연어로 요청하면, Claude가 알맞은 명령을 만들어 deskclaw에 보냅니다.

| Claude Code에 이렇게 말하면 | deskclaw가 받는 명령 (예) |
|---|---|
| "deskclaw@내맥에서 실행 중인 앱 목록 보여줘" | `[AX] apps` |
| "계산기 열어서 7 더하기 5 눌러봐" | `[LAUNCH] Calculator` → `[AX] press Calculator "role=AXButton desc=7"` … |
| "카톡에서 답장해야 할 방만 골라줘" | `[KAKAO] triage -n 60` |
| "'우리팀' 방 최근 대화 20개 읽어줘" | `[KAKAO] read "우리팀" -n 20` |
| "'우리팀' 방 대화 전부 파일로 내보내줘" | `[KAKAO] export "우리팀"` |
| "'우리팀' 방에 '확인했습니다' 보내줘" | `[KAKAO] send "우리팀" "확인했습니다" --yes` → **맥 화면에 허용/거부 창** |

내부적으로는 DureClaw MCP 도구로 보냅니다.

```
mcp__oah__send_task(to: "deskclaw@내맥", instructions: "[KAKAO] unread -n 30")
```

REST로 직접 보낼 수도 있습니다(스크립트·다른 도구에서).

```bash
curl -s -X POST http://<버스 주소>/api/task \
  -H "Authorization: Bearer <버스 토큰>" -H "Content-Type: application/json" \
  -d '{"task_id":"t1","to":"deskclaw@내맥","instructions":"[AX] apps"}'
curl -s http://<버스 주소>/api/task/t1 -H "Authorization: Bearer <버스 토큰>"   # 결과
```

작업이 도는 동안 deskclaw 맥 화면에는 주황 테두리·배너와 **"Claude" 가상 커서**가 보입니다.
작업 중에 연 창(설정 창, 채팅방 등)은 끝나면 자동으로 닫힙니다.

## 6. 한 번 가르치고 반복하기 (RPA)

같은 일을 매번 AI에게 시키지 말고, 처음 한 번만 시킨 뒤 녹화해 두세요.

```
"계산기 테스트를 매크로로 녹화해줘: 열고, 7 + 5 = 누르고, 결과 확인"
```

Claude가 `[RECORD] calc-test` → 단계별 조작 → `[ENDREC]` 순서로 보냅니다. 다음부터는:

```
"calc-test 매크로 실행해줘"      →  [RUN] calc-test   (AI 호출 없이 수 초)
```

`[AX] press/set`으로 녹화한 단계는 버튼을 이름으로 찾기 때문에 창이 움직여도 그대로 재생됩니다.

## 7. 안전장치 한눈에

| 장치 | 동작 |
|---|---|
| 화면 표시 | 조작 중 주황 테두리 + "건드리지 마세요" 배너 + 가상 커서 |
| 승인 창 | 메시지 전송 같은 실제 동작은 맥 화면에서 사람이 **허용**해야 실행 (90초 무응답 = 거부) |
| 노드 쪽 결정 | 원격 마스터는 승인 설정을 바꿀 수 없음 — 설정 창의 "항상 허용 어댑터"로만 |
| 창 정리 | 작업 중 새로 연 창만 끝나면 닫음, 원래 있던 창은 그대로 |
| 일시정지 | 메뉴바 → **일시정지** 한 번이면 노드가 멈춤 |

## 8. 문제 해결

| 증상 | 확인할 것 |
|---|---|
| 아이콘이 계속 흐리거나 사선 | 버스 주소·포트, 버스가 떠 있는지, 같은 네트워크/Tailscale인지. 메뉴바 → **로그 보기** |
| 작업 발행이 401 | 버스 토큰(OAH_SECRET)이 맞는지, `Authorization: Bearer` 헤더 |
| `Accessibility permission missing` | 메뉴바 → **권한 확인** → 손쉬운 사용에서 DeskClaw 켜기 |
| `app not running: …` | 대상 앱을 먼저 실행 (`[LAUNCH] 앱이름`). 한국어 맥이면 `계산기`·`Calculator` 둘 다 됨 |
| `no element matches` | `[AX] tree 앱이름`으로 실제 버튼 이름을 확인. 띄어쓰기 있는 값은 `title=\"채팅방 설정\"`처럼 따옴표 |
| 메뉴바 아이콘이 안 보임 | 노치가 있는 맥에서 메뉴바 아이콘이 많으면 노치 뒤로 숨음 — 다른 아이콘을 줄이기 |
| 다시 빌드한 뒤 권한이 풀림 | Apple Development 인증서가 없으면 임시 서명이라 빌드마다 권한을 다시 켜야 함 |

---

더 자세한 명령 목록은 [README](../README.md), DureClaw 버스 자체는
[DureClaw 저장소](https://github.com/DureClaw/dureclaw)를 참고하세요.

> 카카오톡 이용약관은 자동화 프로그램 사용을 제한합니다. 대량 발송이나 짧은 주기의 반복 실행은 하지 마세요.
