// KakaoTalk (macOS) adapter — read the chat list, triage new messages, read a room, send.
// Ported from tools/kakaotalk (kakao.py + kax): the AX primitives live in axhelper
// (dump / pick / set / key), the KakaoTalk knowledge lives here.
//
// Measured with kax (2026-10-21): list of 60 rooms 2.8s · open room 1.1s · read 0.17s ·
// send 0.33s — vs ~1 min each for a screenshot+click loop.
//
// Notes
//   - KakaoTalk only exposes windows while frontmost, so every call brings it to the front.
//   - read/send OPEN the room → the sender's '1' disappears. rooms/unread/triage don't.
//   - send is a dry-run unless --yes AND the node sets DESK_ALLOW_CONFIRM=KAKAO.
//   - KakaoTalk's terms restrict automation: no bulk sends, no tight polling loops.
package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	kakaoApp  = "com.kakao.KakaoTalkMac"
	kakaoMain = "카카오톡"
	kakaoBox  = "메시지 입력"
)

var kakaoDate = regexp.MustCompile(`^\d{4}년 \d{1,2}월 \d{1,2}일`)

var kakaoTime = regexp.MustCompile(`^(오전|오후) \d{1,2}:\d{2}$|^\d{1,2}월 \d{1,2}일$|^어제$|^\d{4}\. ?\d{1,2}\. ?\d{1,2}\.?$`)

func init() {
	registerApp(&appAdapter{
		verb:        "[KAKAO]",
		description: "카카오톡",
		subcommands: map[string]string{
			"rooms":  "[-n 30] — chat list (doesn't open rooms)",
			"unread": "[-n 60] — rooms with unread messages",
			"triage": "[-n 60] — unread + category / needs_reply / urgency (Jev, else rules)",
			"read":   `"<room>" [-n 20 | --all] — open the room, recent (or all) messages (clears the sender's '1')`,
			"send":   `"<room>" "<text>" [--yes] — dry-run without --yes`,
			"export": `"<room>" — KakaoTalk's own full-history export (CSV: Date, User, Message) → file path`,
		},
		confirmFlag: "--yes",
		run:         kakaoRun,
	})
}

type kakaoRoom struct {
	Name     string `json:"name"`
	Members  string `json:"members,omitempty"`
	Time     string `json:"time,omitempty"`
	Preview  string `json:"preview"`
	Unread   string `json:"unread,omitempty"`
	OpenChat bool   `json:"openchat,omitempty"`
	Me       bool   `json:"me,omitempty"`
	Pinned   bool   `json:"pinned,omitempty"`
	// triage
	Category    string  `json:"category,omitempty"`
	NeedsReply  bool    `json:"needs_reply,omitempty"`
	NeedsReplyP float64 `json:"needs_reply_p,omitempty"`
	Urgency     any     `json:"urgency,omitempty"`
	Engine      string  `json:"engine,omitempty"`
}

type kakaoMsg struct {
	Date   string `json:"date,omitempty"`
	Time   string `json:"time,omitempty"`
	Sender string `json:"sender,omitempty"`
	Text   string `json:"text"`
}

func kakaoRun(sub string, args []string) (string, error) {
	f := parseAppFlags(args, "-n")
	asJSON := f.bools["--json"]
	switch sub {
	case "rooms", "unread", "triage":
		def := map[string]int{"rooms": 30, "unread": 60, "triage": 60}[sub]
		rooms, err := kakaoRooms(f.int("-n", def))
		if err != nil {
			return "", err
		}
		if sub != "rooms" {
			rooms = kakaoUnread(rooms)
		}
		if sub == "triage" {
			rooms = kakaoTriage(rooms)
		}
		if asJSON {
			return toJSON(rooms), nil
		}
		return kakaoFormatRooms(rooms, sub == "triage"), nil

	case "read":
		if len(f.pos) < 1 {
			return "", fmt.Errorf(`needs "<room>"`)
		}
		n := f.int("-n", 20)
		if f.bools["--all"] {
			n = 0
		}
		msgs, err := kakaoRead(f.pos[0], n)
		if err != nil {
			return "", err
		}
		if asJSON {
			return toJSON(msgs), nil
		}
		var lines []string
		for _, m := range msgs {
			lines = append(lines, fmt.Sprintf("[%s %s] %s: %s", m.Date, m.Time, firstNonEmpty(m.Sender, "-"), clip(m.Text, 200)))
		}
		return strings.Join(lines, "\n"), nil

	case "export":
		if len(f.pos) < 1 {
			return "", fmt.Errorf(`needs "<room>"`)
		}
		path, err := kakaoExport(f.pos[0])
		if err != nil {
			return "", err
		}
		return kakaoCSVSummary(path), nil

	case "send":
		if len(f.pos) < 2 {
			return "", fmt.Errorf(`needs "<room>" "<text>"`)
		}
		room, text := f.pos[0], f.pos[1]
		if !f.bools["--yes"] {
			return fmt.Sprintf("[dry-run] '%s' 에 보낼 내용: %s\n실제 전송은 --yes 를 붙이세요.", room, text), nil
		}
		if err := kakaoSend(room, text); err != nil {
			return "", err
		}
		return "전송 완료: " + room, nil
	}
	return "", fmt.Errorf("unknown subcommand %s", sub)
}

// ── read ────────────────────────────────────────────────────────────────────

func kakaoRooms(n int) ([]kakaoRoom, error) {
	nodes, err := axDump(kakaoApp, "--window", kakaoMain, "--rows", strconv.Itoa(n), "--depth", "20")
	if err != nil {
		return nil, err
	}
	var rooms []kakaoRoom
	for _, row := range axRows(nodes) {
		// '조용한 채팅방' is the folder row that groups muted rooms
		if r, ok := kakaoParseRoom(row); ok && r.Name != "조용한 채팅방" {
			rooms = append(rooms, r)
		}
	}
	return rooms, nil
}

func kakaoUnread(rooms []kakaoRoom) []kakaoRoom {
	var out []kakaoRoom
	for _, r := range rooms {
		if r.Unread != "" {
			out = append(out, r)
		}
	}
	return out
}

func kakaoParseRoom(row []axNode) (kakaoRoom, bool) {
	var r kakaoRoom
	for _, n := range row {
		switch n.R {
		case "AXStaticText":
			switch {
			case r.Name == "":
				r.Name = n.V
			case kakaoTime.MatchString(n.V):
				r.Time = n.V
			case isCount(n.V):
				// a number before the time = group member count, after it = unread count
				if r.Time == "" {
					r.Members = n.V
				} else {
					r.Unread = n.V
				}
			}
		case "AXTextArea":
			r.Preview = n.V
		case "AXImage":
			r.OpenChat = r.OpenChat || strings.Contains(n.Desc, "badge openchat")
			r.Me = r.Me || strings.Contains(n.Desc, "badge me")
			r.Pinned = r.Pinned || strings.Contains(n.Desc, "icon pin")
		}
	}
	return r, r.Name != ""
}

func isCount(s string) bool {
	s = strings.TrimSuffix(s, "+")
	_, err := strconv.Atoi(s)
	return s != "" && err == nil
}

func kakaoOpen(room string) error {
	_, err := axRun([]string{"pick", kakaoApp, room, "--window", kakaoMain, "--rows", "200"})
	if err != nil {
		return err
	}
	return nil
}

// kakaoRead returns the last n messages; n <= 0 loads the whole history first (see below).
func kakaoRead(room string, n int) ([]kakaoMsg, error) {
	if err := kakaoOpen(room); err != nil {
		return nil, err
	}
	if n <= 0 {
		rows, err := kakaoLoadHistory(room)
		if err != nil {
			return nil, err
		}
		n = rows
	}
	nodes, err := axDump(kakaoApp, "--window", room, "--rows", strconv.Itoa(n), "--tail", "--depth", "20")
	if err != nil {
		return nil, err
	}
	var msgs []kakaoMsg
	date := ""
	for _, row := range axRows(nodes) {
		m, ok := kakaoParseMsg(row)
		if m.Date != "" {
			date = m.Date
		}
		if ok {
			m.Date = date
			msgs = append(msgs, m)
		}
	}
	return msgs, nil
}

// kakaoLoadHistory: KakaoTalk keeps only recent messages loaded and fetches ~60 older ones each
// time the chat is scrolled to the top. Scroll up until the row count stops growing (or a cap),
// then return the row count.
func kakaoLoadHistory(room string) (int, error) {
	const maxRows, stableNeeded = 20000, 3
	count := func() (int, error) {
		out, err := axRun([]string{"rowcount", kakaoApp, "--window", room})
		if err != nil {
			return 0, err
		}
		return strconv.Atoi(strings.TrimSpace(out))
	}
	last, err := count()
	if err != nil {
		return 0, err
	}
	visCaption("⇡ 이전 대화 불러오는 중…")
	for stable := 0; stable < stableNeeded && last < maxRows; {
		if _, err := axRun([]string{"scroll", kakaoApp, "0", "--window", room, "--rows", "0"}); err != nil {
			return 0, err
		}
		time.Sleep(900 * time.Millisecond)
		n, err := count()
		if err != nil {
			return 0, err
		}
		if n > last {
			stable = 0
			visCaption(fmt.Sprintf("⇡ 이전 대화 불러오는 중… %d줄", n))
		} else {
			stable++
		}
		last = n
	}
	return last, nil
}

// kakaoParseMsg: received messages carry the sender's name (first AXStaticText of a run);
// my own messages and follow-ups from the same sender have none. The time text may carry the
// unread count on its first line ("29\n오후 11:07"). Date-separator rows set m.Date only.
func kakaoParseMsg(row []axNode) (kakaoMsg, bool) {
	var m kakaoMsg
	var text []string
	for _, n := range row {
		lines := strings.Split(n.V, "\n")
		last := strings.TrimSpace(lines[len(lines)-1])
		switch {
		case n.R == "AXStaticText" && kakaoTime.MatchString(last):
			m.Time = last
		case n.R == "AXStaticText" && kakaoDate.MatchString(n.V):
			m.Date = n.V
		case n.R == "AXStaticText" && m.Sender == "":
			m.Sender = n.V
		case n.R == "AXTextArea" && n.V != "":
			text = append(text, n.V)
		}
	}
	m.Text = strings.Join(text, "\n")
	return m, len(text) > 0
}

// ── export ──────────────────────────────────────────────────────────────────

// kakaoExport drives KakaoTalk's own "대화 내용 저장 → 텍스트 파일로 저장" (complete history,
// unlike scrolling, which KakaoTalk caps): 채팅 menu › 채팅방 설정 › 대화 내용 관리 tab (the 2nd,
// unlabeled sidebar button) › 텍스트 파일로 저장 › 저장 › "내보내기가 완료되었습니다" 확인.
// Saves to the panel's default folder (usually Downloads).
func kakaoExport(room string) (string, error) {
	const settings = "Window" // KakaoTalk's settings window has no real title
	started := time.Now()
	// a settings window left open would own the 채팅 menu — close it first
	if ws, err := axRun([]string{"windows", kakaoApp}); err == nil && slices.Contains(strings.Split(ws, "\n"), settings) {
		_, _ = axRun([]string{"close", kakaoApp, "--window", settings, "--rows", "0"})
		time.Sleep(400 * time.Millisecond)
	}
	if err := kakaoOpen(room); err != nil {
		return "", err
	}
	if o, code := doAX(shellJoin([]string{"press", kakaoApp, `role=AXMenuItem title="채팅방 설정"`,
		"--menus", "--window", room, "--depth", "4", "--rows", "0"})); code != 0 {
		return "", fmt.Errorf("채팅방 설정 열기: %s", o)
	}
	time.Sleep(800 * time.Millisecond)
	defer axRun([]string{"close", kakaoApp, "--window", settings, "--rows", "0", "--no-activate"})
	for _, s := range [][]string{
		{"press", kakaoApp, "path=0.2", "--window", settings, "--rows", "0"}, // 대화 내용 관리 tab
		{"press", kakaoApp, `role=AXButton title="텍스트 파일로 저장"`, "--window", settings, "--rows", "0"},
		{"press", kakaoApp, "role=AXButton title=저장", "--window", settings, "--rows", "0", "--depth", "10"},
	} {
		if _, code := doAX(shellJoin(s)); code != 0 {
			return "", fmt.Errorf("%s 실패", s[2])
		}
		time.Sleep(900 * time.Millisecond)
	}
	// "대화내용 내보내기가 완료되었습니다" sheet — dismiss it (it also blocks closing the window)
	for i := 0; i < 20; i++ {
		if _, code := doAX(shellJoin([]string{"press", kakaoApp, "role=AXButton title=확인", "--window", settings,
			"--rows", "0", "--depth", "10"})); code == 0 {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	// find the file the save panel wrote: newest KakaoTalk_Chat_* since we started. (Not matched
	// by room name — the file name's Hangul may be NFD-normalized and differ byte-wise.)
	home, _ := os.UserHomeDir()
	for i := 0; i < 30; i++ {
		var newest string
		var newestT time.Time
		for _, dir := range []string{"Downloads", "Documents", "Desktop"} {
			matches, _ := filepath.Glob(filepath.Join(home, dir, "KakaoTalk_Chat_*"))
			for _, m := range matches {
				if st, err := os.Stat(m); err == nil && st.ModTime().After(started.Add(-2*time.Second)) && st.ModTime().After(newestT) {
					newest, newestT = m, st.ModTime()
				}
			}
		}
		if newest != "" {
			return newest, nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return "", fmt.Errorf("저장된 파일을 찾지 못함 (Downloads/Documents/Desktop)")
}

// kakaoCSVSummary: path + message count + date range of an exported chat.
func kakaoCSVSummary(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return "saved " + path
	}
	defer f.Close()
	rows, err := csv.NewReader(f).ReadAll()
	if err != nil || len(rows) < 2 {
		return "saved " + path
	}
	rows[0][0] = strings.TrimPrefix(rows[0][0], "\ufeff") // KakaoTalk writes a UTF-8 BOM
	users := map[string]bool{}
	for _, r := range rows[1:] {
		if len(r) > 1 {
			users[r[1]] = true
		}
	}
	return fmt.Sprintf("saved %s\n메시지 %d개 · %s → %s · 보낸 사람 %d명 (열: %s)",
		path, len(rows)-1, rows[1][0], rows[len(rows)-1][0], len(users), strings.Join(rows[0], ", "))
}

// shellJoin quotes args so doAX/splitArgs read them back unchanged.
func shellJoin(args []string) string {
	q := make([]string, len(args))
	for i, a := range args {
		q[i] = `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(a) + `"`
	}
	return strings.Join(q, " ")
}

// ── send ────────────────────────────────────────────────────────────────────

func kakaoSend(room, text string) error {
	if err := kakaoOpen(room); err != nil {
		return err
	}
	// set reads the value back, so a silently dropped input fails here
	if _, err := axRun([]string{"set", kakaoApp, fmt.Sprintf("desc=%q", kakaoBox), text, "--window", room, "--rows", "0"}); err != nil {
		return err
	}
	if _, err := axRun([]string{"key", kakaoApp, "return"}); err != nil {
		return err
	}
	time.Sleep(300 * time.Millisecond)
	nodes, err := axDump(kakaoApp, "--window", room, "--rows", "3", "--tail", "--depth", "20")
	if err != nil {
		return err
	}
	sent := false
	for _, n := range nodes {
		if n.Desc == kakaoBox && n.V != "" {
			return fmt.Errorf("input box not empty after Return — send unconfirmed")
		}
		sent = sent || (n.R == "AXTextArea" && n.V == text)
	}
	if !sent {
		return fmt.Errorf("sent text not found among the latest messages — send unconfirmed")
	}
	return nil
}

// ── triage ──────────────────────────────────────────────────────────────────

var kakaoCategories = map[string]string{
	"personal":  "가족·지인 등 실제 사람과의 1:1 또는 소규모 대화",
	"work":      "업무·프로젝트·고객·거래처 관련 대화",
	"community": "다수가 참여하는 오픈채팅·커뮤니티 단체방",
	"ad":        "기업·서비스 채널의 광고·마케팅·알림 메시지",
	"self":      "나와의 채팅(메모용)",
}

func kakaoTriage(rooms []kakaoRoom) []kakaoRoom {
	useJev := jevAvailable()
	out := make([]kakaoRoom, len(rooms))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 8) // one Jev call per room, 8 in flight
	for i, r := range rooms {
		wg.Add(1)
		go func(i int, r kakaoRoom) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			if useJev {
				if j, err := kakaoJevTriage(r); err == nil {
					out[i] = j
					return
				}
			}
			out[i] = kakaoRuleTriage(r)
		}(i, r)
	}
	wg.Wait()
	sort.SliceStable(out, func(a, b int) bool {
		if out[a].NeedsReplyP != out[b].NeedsReplyP {
			return out[a].NeedsReplyP > out[b].NeedsReplyP
		}
		return out[a].Category < out[b].Category
	})
	return out
}

func kakaoRuleTriage(r kakaoRoom) kakaoRoom {
	p := r.Preview
	switch {
	case r.Me:
		r.Category = "self"
	case strings.Contains(p, "(광고)") || strings.HasPrefix(p, "[") || strings.HasPrefix(p, "#") ||
		strings.Contains(p, "팀입니다") || strings.Contains(p, "님,"):
		r.Category = "ad"
	case r.OpenChat || r.Members != "":
		r.Category = "community"
	default:
		r.Category = "personal"
	}
	r.NeedsReply = (r.Category == "personal" || r.Category == "work") && r.Unread != ""
	r.NeedsReplyP = 0
	if r.NeedsReply {
		r.NeedsReplyP = 1
	}
	r.Engine = "rule"
	return r
}

func kakaoJevTriage(r kakaoRoom) (kakaoRoom, error) {
	state := map[string]any{
		"room": r.Name, "is_openchat": r.OpenChat, "is_self_chat": r.Me,
		"member_count": r.Members, "unread": r.Unread, "last_time": r.Time, "last_message": r.Preview,
	}
	a, err := jevSystemOne(state, map[string]map[string]any{
		"category":    jevChoice("이 카카오톡 채팅방의 성격", kakaoCategories),
		"needs_reply": jevNoul("내가(방 주인이) 직접 답장해야 하는 새 메시지가 와 있다"),
		"urgency":     jevScore("답장의 시급성", []string{"급하지 않음", "오늘 안에", "지금 바로"}),
	})
	if err != nil {
		return r, err
	}
	r.Category = a["category"].Choice
	p := a["needs_reply"].Noul
	// judged from a one-line preview, so needs_reply clusters near 0.5 → only trust it for people chats
	r.NeedsReply = (r.Category == "personal" || r.Category == "work") && p >= 0.6
	r.NeedsReplyP = float64(int(p*1000)) / 1000
	r.Urgency = a["urgency"].Score
	r.Engine = "jev"
	return r, nil
}

// ── output ──────────────────────────────────────────────────────────────────

func kakaoFormatRooms(rooms []kakaoRoom, triaged bool) string {
	var lines []string
	for i, r := range rooms {
		tag := ""
		if r.Me {
			tag += "[나]"
		}
		if r.OpenChat {
			tag += "[오픈]"
		}
		extra := ""
		if triaged {
			extra = " | " + r.Category
			if r.NeedsReply {
				extra += " ✉답장필요"
			}
		}
		preview := []rune(strings.ReplaceAll(r.Preview, "\n", " "))
		if len(preview) > 40 {
			preview = preview[:40]
		}
		lines = append(lines, fmt.Sprintf("%2d. %s%s | %s | 안읽음 %s%s | %s",
			i+1, r.Name, tag, r.Time, firstNonEmpty(r.Unread, "0"), extra, string(preview)))
	}
	if triaged {
		engine := "규칙 기반 (TYPESAFE_API_KEY 없음)"
		if jevAvailable() {
			engine = "Jev"
		}
		lines = append(lines, "", "분류 엔진: "+engine)
	}
	return strings.Join(lines, "\n")
}

func toJSON(v any) string {
	b, _ := json.MarshalIndent(v, "", "  ")
	return string(b)
}
