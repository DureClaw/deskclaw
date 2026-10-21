// DeskClaw.app — macOS menu-bar shell for the deskclaw node.
//
// Runs the Go node (Contents/MacOS/deskclaw-node) as a child with DESK_UI=1 and talks to it over
// JSON lines (see ui.go): shows connection state in the menu bar, keeps a recent-task list,
// posts notifications, and asks the person at the machine to approve real-world actions
// (e.g. `[KAKAO] send … --yes`). Settings live in ~/Library/Application Support/deskclaw/
// config.json (env vars for the node). Being an .app also gives macOS permissions
// (Accessibility, Screen Recording) one stable owner instead of whichever terminal ran it.
import AppKit
import ServiceManagement
import UserNotifications

let supportDir = FileManager.default.homeDirectoryForCurrentUser
    .appendingPathComponent("Library/Application Support/deskclaw")
let configURL = supportDir.appendingPathComponent("config.json")
let logURL = FileManager.default.homeDirectoryForCurrentUser.appendingPathComponent("Library/Logs/DeskClaw/deskclaw.log")

/// Settings fields → node env vars. (label, env key, secret, placeholder)
let fields: [(String, String, Bool, String)] = [
    ("버스 서버", "STATE_SERVER", false, "100.x.x.x:4000"),
    ("버스 토큰", "OAH_SECRET", true, ""),
    ("Work Key", "WORK_KEY", false, "WK-…"),
    ("에이전트 이름", "AGENT_NAME", false, "deskclaw@\(Host.current().localizedName ?? "mac")"),
    ("Brain URL", "BRAIN_URL", false, "http://master:4111"),
    ("Brain 토큰", "BRAIN_TOKEN", true, ""),
    ("항상 허용 어댑터", "DESK_ALLOW_CONFIRM", false, "비워두면 매번 묻기 (예: KAKAO)"),
    ("Jev 키 (선택)", "TYPESAFE_API_KEY", true, "카카오 분류용"),
]

func loadConfig() -> [String: String] {
    guard let d = try? Data(contentsOf: configURL),
          let m = try? JSONSerialization.jsonObject(with: d) as? [String: String] else { return [:] }
    return m
}

func saveConfig(_ m: [String: String]) {
    try? FileManager.default.createDirectory(at: supportDir, withIntermediateDirectories: true)
    if let d = try? JSONSerialization.data(withJSONObject: m, options: [.prettyPrinted, .sortedKeys]) {
        try? d.write(to: configURL)
        try? FileManager.default.setAttributes([.posixPermissions: 0o600], ofItemAtPath: configURL.path)  // holds tokens
    }
}

struct TaskRow { let id: String; var text: String; var status: String }

enum ClawLook { case solid, faint, off, busy }

/// Menu-bar template icon: the app icon's three talons. solid = connected, faint = connecting,
/// off = faint + slash (disconnected/stopped), busy = solid + dot (a task is running).
func clawIcon(_ look: ClawLook) -> NSImage {
    let img = NSImage(size: NSSize(width: 18, height: 18), flipped: false) { _ in
        let alpha: CGFloat = (look == .solid || look == .busy) ? 1 : 0.45
        NSColor.black.withAlphaComponent(alpha).setFill()
        for (i, x) in [CGFloat(4), 9, 14].enumerated() {
            let tip: CGFloat = i == 1 ? 2 : 4
            let t = NSBezierPath()
            t.move(to: NSPoint(x: x - 2.6, y: 17))
            t.curve(to: NSPoint(x: x + 1.8, y: tip), controlPoint1: NSPoint(x: x - 4.2, y: 10), controlPoint2: NSPoint(x: x - 2, y: 5))
            t.curve(to: NSPoint(x: x + 1.4, y: 17), controlPoint1: NSPoint(x: x + 0.2, y: 7), controlPoint2: NSPoint(x: x + 1.4, y: 12))
            t.close()
            t.fill()
        }
        if look == .off {
            NSColor.black.setStroke()
            let slash = NSBezierPath()
            slash.move(to: NSPoint(x: 2, y: 16)); slash.line(to: NSPoint(x: 16, y: 2))
            slash.lineWidth = 1.8
            slash.lineCapStyle = .round
            slash.stroke()
        }
        if look == .busy {
            NSColor.black.setFill()
            NSBezierPath(ovalIn: NSRect(x: 12.5, y: 0, width: 5.5, height: 5.5)).fill()
        }
        return true
    }
    img.isTemplate = true
    return img
}

final class App: NSObject, NSApplicationDelegate, NSWindowDelegate {
    let item = NSStatusBar.system.statusItem(withLength: NSStatusItem.variableLength)
    var node: Process?
    var stdin: FileHandle?
    var buffer = Data()
    var state = "stopped", detail = ""
    var paused = false
    var tasks: [TaskRow] = []
    var running = 0
    var wasConnected = false
    var settings: NSWindow?
    var inputs: [String: NSTextField] = [:]
    var overlayBox: NSButton?
    var notifyAll: Bool {
        get { UserDefaults.standard.bool(forKey: "notifyAll") }
        set { UserDefaults.standard.set(newValue, forKey: "notifyAll") }
    }

    func applicationDidFinishLaunching(_ n: Notification) {
        UNUserNotificationCenter.current().requestAuthorization(options: [.alert, .sound]) { _, _ in }
        try? FileManager.default.createDirectory(at: logURL.deletingLastPathComponent(), withIntermediateDirectories: true)
        render()
        if loadConfig()["STATE_SERVER", default: ""].isEmpty {
            openSettings()  // first run
        } else {
            startNode()
        }
    }

    func applicationWillTerminate(_ n: Notification) { stopNode() }

    // ── node process ─────────────────────────────────────────────────────────

    func startNode() {
        guard node == nil, !paused else { return }
        let dir = Bundle.main.bundleURL.appendingPathComponent("Contents/MacOS")
        let p = Process()
        p.executableURL = dir.appendingPathComponent("deskclaw-node")
        var env = ProcessInfo.processInfo.environment
        for (k, v) in loadConfig() where !v.isEmpty { env[k] = v }
        env["DESK_UI"] = "1"
        env["DESK_AX_HELPER"] = dir.appendingPathComponent("axhelper").path
        p.environment = env

        let out = Pipe(), inp = Pipe()
        p.standardOutput = out
        p.standardInput = inp
        if !FileManager.default.fileExists(atPath: logURL.path) {
            FileManager.default.createFile(atPath: logURL.path, contents: nil)
        }
        if let log = try? FileHandle(forWritingTo: logURL) {
            log.seekToEndOfFile()
            p.standardError = log
        }
        out.fileHandleForReading.readabilityHandler = { [weak self] h in
            let d = h.availableData
            DispatchQueue.main.async { self?.consume(d) }
        }
        p.terminationHandler = { [weak self] proc in
            DispatchQueue.main.async {
                guard let self, self.node === proc else { return }
                self.node = nil
                self.stdin = nil
                self.setState("stopped", "노드가 종료됨 (코드 \(proc.terminationStatus))")
                if !self.paused {  // crashed → restart
                    DispatchQueue.main.asyncAfter(deadline: .now() + 3) { self.startNode() }
                }
            }
        }
        do {
            try p.run()
            node = p
            stdin = inp.fileHandleForWriting
            setState("connecting", env["STATE_SERVER"] ?? "")
        } catch {
            setState("stopped", "실행 실패: \(error.localizedDescription)")
        }
    }

    func stopNode() {
        guard let p = node else { return }
        node = nil
        stdin = nil
        p.terminate()
        setState("stopped", paused ? "일시정지됨" : "")
    }

    func restartNode() {
        stopNode()
        DispatchQueue.main.asyncAfter(deadline: .now() + 0.5) { self.startNode() }
    }

    // ── events from the node ─────────────────────────────────────────────────

    func consume(_ d: Data) {
        buffer.append(d)
        while let nl = buffer.firstIndex(of: 0x0A) {
            let line = buffer.subdata(in: buffer.startIndex..<nl)
            buffer.removeSubrange(buffer.startIndex...nl)
            if let ev = try? JSONSerialization.jsonObject(with: line) as? [String: Any] { handle(ev) }
        }
    }

    func handle(_ ev: [String: Any]) {
        let s = { (k: String) in ev[k] as? String ?? "" }
        switch s("event") {
        case "status":
            setState(s("state"), s("detail"))
        case "task":
            running += 1
            tasks.insert(TaskRow(id: s("id"), text: s("text"), status: "running"), at: 0)
            tasks = Array(tasks.prefix(15))
            render()
        case "result":
            running = max(0, running - 1)
            if let i = tasks.firstIndex(where: { $0.id == s("id") }) { tasks[i].status = s("status") }
            let row = tasks.first { $0.id == s("id") }?.text ?? ""
            if s("status") == "blocked" {
                notify("작업 실패", "\(row)\n\(s("text"))")
            } else if notifyAll {
                notify("작업 완료", row)
            }
            render()
        case "approve":
            approve(id: s("id"), text: s("text"))
        default:
            break
        }
    }

    func setState(_ st: String, _ d: String) {
        if st == "connected" { wasConnected = true }
        if st == "disconnected" && state == "connected" && wasConnected {
            notify("버스 연결 끊김", d)  // only on the transition, not on every retry
        }
        state = st
        detail = d
        render()
    }

    func approve(id: String, text: String) {
        notify("승인 요청", text)
        NSApp.activate(ignoringOtherApps: true)
        let a = NSAlert()
        a.messageText = "DeskClaw 승인 요청"
        a.informativeText = "원격 마스터가 실제 동작을 요청했습니다:\n\n\(text)\n\n허용하면 그대로 실행됩니다. (90초 안에 응답하지 않으면 거부)"
        a.alertStyle = .warning
        a.addButton(withTitle: "거부")
        a.addButton(withTitle: "허용")
        let ok = a.runModal() == .alertSecondButtonReturn
        let reply = "{\"approve\":\"\(id)\",\"ok\":\(ok)}\n"
        stdin?.write(reply.data(using: .utf8)!)
    }

    func notify(_ title: String, _ body: String) {
        let c = UNMutableNotificationContent()
        c.title = "DeskClaw · \(title)"
        c.body = String(body.prefix(240))
        UNUserNotificationCenter.current().add(UNNotificationRequest(identifier: UUID().uuidString, content: c, trigger: nil))
    }

    // ── menu ─────────────────────────────────────────────────────────────────

    func render() {
        let (look, label): (ClawLook, String) = {
            if running > 0 { return (.busy, "작업 중") }
            switch state {
            case "connected": return (.solid, "연결됨")
            case "connecting": return (.faint, "연결 중…")
            case "disconnected": return (.off, "연결 끊김")
            default: return (.off, paused ? "일시정지" : "중지됨")
            }
        }()
        item.button?.image = clawIcon(look)
        item.button?.image?.accessibilityDescription = "DeskClaw \(label)"

        let m = NSMenu()
        m.addItem(disabled("DeskClaw — \(label)"))
        if !detail.isEmpty { m.addItem(disabled(String(detail.prefix(70)))) }
        m.addItem(.separator())

        let recent = NSMenuItem(title: "최근 작업", action: nil, keyEquivalent: "")
        let sub = NSMenu()
        if tasks.isEmpty { sub.addItem(disabled("아직 없음")) }
        for t in tasks {
            let mark = t.status == "running" ? "▶" : t.status == "blocked" ? "✗" : "✓"
            let line = t.text.replacingOccurrences(of: "\n", with: " ")
            sub.addItem(disabled("\(mark) \(line.prefix(60))"))
        }
        recent.submenu = sub
        m.addItem(recent)
        m.addItem(.separator())

        m.addItem(action(paused ? "재개" : "일시정지", #selector(togglePause)))
        m.addItem(action("설정…", #selector(openSettings), key: ","))
        let all = action("모든 작업 완료 알림", #selector(toggleNotifyAll))
        all.state = notifyAll ? .on : .off
        m.addItem(all)
        let login = action("로그인 시 자동 실행", #selector(toggleLogin))
        login.state = SMAppService.mainApp.status == .enabled ? .on : .off
        m.addItem(login)
        m.addItem(action("권한 확인 (손쉬운 사용·화면 기록)", #selector(checkPermissions)))
        m.addItem(.separator())
        m.addItem(action("로그 보기", #selector(openLog)))
        m.addItem(action("설정 폴더 열기 (매크로·플러그인)", #selector(openSupport)))
        m.addItem(.separator())
        m.addItem(action("DeskClaw 종료", #selector(quit), key: "q"))
        item.menu = m
    }

    func disabled(_ t: String) -> NSMenuItem {
        let i = NSMenuItem(title: t, action: nil, keyEquivalent: "")
        i.isEnabled = false
        return i
    }

    func action(_ t: String, _ sel: Selector, key: String = "") -> NSMenuItem {
        let i = NSMenuItem(title: t, action: sel, keyEquivalent: key)
        i.target = self
        return i
    }

    @objc func togglePause() {
        paused.toggle()
        paused ? stopNode() : startNode()
        render()
    }

    @objc func toggleNotifyAll() { notifyAll.toggle(); render() }

    @objc func toggleLogin() {
        do {
            if SMAppService.mainApp.status == .enabled { try SMAppService.mainApp.unregister() }
            else { try SMAppService.mainApp.register() }
        } catch {
            notify("자동 실행 설정 실패", error.localizedDescription)
        }
        render()
    }

    @objc func checkPermissions() {
        let ax = AXIsProcessTrustedWithOptions([kAXTrustedCheckOptionPrompt.takeUnretainedValue() as String: true] as CFDictionary)
        let screen = CGPreflightScreenCaptureAccess() || CGRequestScreenCaptureAccess()
        let a = NSAlert()
        a.messageText = "DeskClaw 권한"
        a.informativeText = "손쉬운 사용: \(ax ? "허용됨 ✅" : "필요 ❌")\n화면 기록: \(screen ? "허용됨 ✅" : "필요 ❌")\n\n" +
            "시스템 설정 › 개인정보 보호 및 보안 에서 DeskClaw를 켜 주세요. 켠 뒤에는 노드를 재시작합니다."
        NSApp.activate(ignoringOtherApps: true)
        a.runModal()
        if ax && screen { restartNode() }
    }

    @objc func openLog() { NSWorkspace.shared.open(logURL) }
    @objc func openSupport() {
        try? FileManager.default.createDirectory(at: supportDir, withIntermediateDirectories: true)
        NSWorkspace.shared.open(supportDir)
    }
    @objc func quit() { NSApp.terminate(nil) }

    // ── settings window ──────────────────────────────────────────────────────

    @objc func openSettings() {
        if let w = settings {
            NSApp.activate(ignoringOtherApps: true)
            w.makeKeyAndOrderFront(nil)
            return
        }
        let cfg = loadConfig()
        var rows: [[NSView]] = []
        for (label, key, secret, placeholder) in fields {
            let f: NSTextField = secret ? NSSecureTextField() : NSTextField()
            f.stringValue = cfg[key] ?? ""
            f.placeholderString = placeholder
            f.widthAnchor.constraint(equalToConstant: 320).isActive = true
            inputs[key] = f
            rows.append([NSTextField(labelWithString: label), f])
        }
        let ov = NSButton(checkboxWithTitle: "작업 중 화면 표시 (주황 테두리·배너)", target: nil, action: nil)
        ov.state = cfg["DESK_OVERLAY"] == "0" ? .off : .on
        overlayBox = ov
        rows.append([NSView(), ov])

        let grid = NSGridView(views: rows)
        grid.column(at: 0).xPlacement = .trailing
        grid.rowSpacing = 8
        let save = NSButton(title: "저장하고 연결", target: self, action: #selector(saveSettings))
        save.keyEquivalent = "\r"
        let note = NSTextField(wrappingLabelWithString: "토큰은 ~/Library/Application Support/deskclaw/config.json 에 저장됩니다 (본인만 읽기 가능).")
        note.font = .systemFont(ofSize: 11)
        note.textColor = .secondaryLabelColor
        let stack = NSStackView(views: [grid, note, save])
        stack.orientation = .vertical
        stack.alignment = .trailing
        stack.spacing = 14
        stack.edgeInsets = NSEdgeInsets(top: 20, left: 20, bottom: 20, right: 20)

        let w = NSWindow(contentRect: .zero, styleMask: [.titled, .closable], backing: .buffered, defer: false)
        w.title = "DeskClaw 설정"
        w.contentView = stack
        w.isReleasedWhenClosed = false
        w.delegate = self
        w.center()
        settings = w
        NSApp.activate(ignoringOtherApps: true)
        w.makeKeyAndOrderFront(nil)
    }

    @objc func saveSettings() {
        var cfg = loadConfig()
        for (_, key, _, _) in fields {
            cfg[key] = inputs[key]?.stringValue.trimmingCharacters(in: .whitespaces) ?? ""
        }
        cfg["DESK_OVERLAY"] = overlayBox?.state == .on ? "" : "0"
        saveConfig(cfg)
        settings?.close()
        paused = false
        restartNode()
    }

    func windowWillClose(_ n: Notification) {
        settings = nil
        inputs = [:]
    }
}

let app = NSApplication.shared
let delegate = App()
app.delegate = delegate
app.setActivationPolicy(.accessory)
app.run()
