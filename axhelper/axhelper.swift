// axhelper — universal macOS Accessibility hands for deskclaw. Drives ANY app by element
// (role/title/description/value), not pixels — so steps survive window moves and resizes.
// Generalised from the KakaoTalk-only `kax`: same speed tricks (batch attribute reads,
// bounded table rows), no app-specific code.
//
//   axhelper apps                              running GUI apps (name · bundle id · pid)
//   axhelper tree  <app> [opts]                element tree, one line per node: path role "label"
//   axhelper find  <app> <query> [opts]        matching elements (path + label + frame)
//   axhelper text  <app> [query] [opts]        all text under the match (or the windows)
//   axhelper press <app> <query> [opts]        AXPress the match (falls back to a real click)
//   axhelper set   <app> <query> <value> [opts] set AXValue, read back to confirm
//   axhelper dump  <app> [opts]                tree as JSON lines {d,r,t,v,desc} — for app adapters
//   axhelper windows <app>                     window titles
//   axhelper pick  <app> <text> [opts]         select the first table row whose first text == <text>,
//                                              press Return, wait for a window titled <text>
//   axhelper key   <app> <name>                post a key (return, escape, tab, …) to the app's pid
//   axhelper overlay "<message>" [seconds=120] on-screen "automation running" border + banner
//                                              (click-through, no focus; exits on SIGTERM / parent death)
//
// <app>    app name ("Safari"), bundle id ("com.apple.Safari") or pid
// <query>  space-separated terms, all must match:
//            plain word        label contains it (case-insensitive; label = title|desc|value)
//            role=AXButton     exact role      title=… desc=… value=…  exact attribute
//            title~… desc~…    attribute contains
//            #2                take the 2nd match (default 1st)
//            path=0.1.3        address an element directly (from tree/find output)
//            desc="two words"  double quotes group a value containing spaces
// opts     --depth N (12)  --max N (200 nodes printed)  --rows N (30 table rows; 0 skips rows)
//          --tail (last rows)  --window <title> (exact, else contains)  --menus (include menu bar)
//          --no-activate
import AppKit
import ApplicationServices
import Foundation

func fail(_ msg: String, _ code: Int32 = 1) -> Never {
    FileHandle.standardError.write((msg + "\n").data(using: .utf8)!)
    exit(code)
}

func attr(_ el: AXUIElement, _ name: String) -> AnyObject? {
    var v: AnyObject?
    return AXUIElementCopyAttributeValue(el, name as CFString, &v) == .success ? v : nil
}

func str(_ o: AnyObject?) -> String? {
    if let s = o as? String { return s }
    if let n = o as? NSNumber { return n.stringValue }
    return nil
}

// ---------------------------------------------------------------- args

var positional: [String] = []
var opts: [String: String] = [:]
var flags = Set<String>()
do {
    var it = CommandLine.arguments.dropFirst().makeIterator()
    while let a = it.next() {
        switch a {
        case "--depth", "--max", "--rows", "--window": opts[a] = it.next() ?? ""
        case "--tail", "--menus", "--no-activate": flags.insert(a)
        default: positional.append(a)
        }
    }
}
let maxDepth = Int(opts["--depth"] ?? "") ?? 12
let maxPrint = Int(opts["--max"] ?? "") ?? 200
let maxRows = Int(opts["--rows"] ?? "") ?? 30

// ---------------------------------------------------------------- node

let ATTRS = [kAXRoleAttribute, kAXTitleAttribute, kAXDescriptionAttribute, kAXValueAttribute,
             kAXChildrenAttribute] as CFArray

struct Node {
    let el: AXUIElement
    let path: String
    let depth: Int
    var role = "", title = "", desc = "", value = ""
    var kids: [AXUIElement] = []

    init(_ el: AXUIElement, _ path: String, _ depth: Int) {
        self.el = el; self.path = path; self.depth = depth
        var raw: CFArray?
        if AXUIElementCopyMultipleAttributeValues(el, ATTRS, AXCopyMultipleAttributeOptions(rawValue: 0), &raw) == .success,
           let v = raw as? [AnyObject], v.count == 5 {
            role = str(v[0]) ?? ""; title = str(v[1]) ?? ""; desc = str(v[2]) ?? ""; value = str(v[3]) ?? ""
            kids = (v[4] as? [AXUIElement]) ?? []
        }
        if role == "AXTable" || role == "AXOutline" {  // big lists: only the rows we need
            let rows = (attr(el, kAXRowsAttribute) as? [AXUIElement]) ?? kids
            kids = flags.contains("--tail") ? Array(rows.suffix(maxRows)) : Array(rows.prefix(maxRows))
        }
    }

    var label: String { [title, desc, value].filter { !$0.isEmpty }.joined(separator: " · ") }

    var line: String {
        let l = label.replacingOccurrences(of: "\n", with: " ")
        let short = l.count > 80 ? String(l.prefix(80)) + "…" : l
        return "\(path) \(role)" + (short.isEmpty ? "" : " \"\(short)\"")
    }

    var frame: String {
        guard let p = attr(el, kAXPositionAttribute), let s = attr(el, kAXSizeAttribute) else { return "" }
        var pt = CGPoint.zero, sz = CGSize.zero
        AXValueGetValue(p as! AXValue, .cgPoint, &pt)
        AXValueGetValue(s as! AXValue, .cgSize, &sz)
        return " @\(Int(pt.x)),\(Int(pt.y)) \(Int(sz.width))x\(Int(sz.height))"
    }
}

// ---------------------------------------------------------------- app & roots

func resolveApp(_ q: String) -> NSRunningApplication {
    let apps = NSWorkspace.shared.runningApplications.filter { $0.activationPolicy == .regular }
    if let pid = Int32(q), let a = NSRunningApplication(processIdentifier: pid) { return a }
    if let a = apps.first(where: { $0.bundleIdentifier == q }) { return a }
    if let a = apps.first(where: { $0.localizedName?.caseInsensitiveCompare(q) == .orderedSame }) { return a }
    // localized UIs rename apps ("Calculator" → "계산기"); the bundle file name stays English
    if let a = apps.first(where: { $0.bundleURL?.deletingPathExtension().lastPathComponent.caseInsensitiveCompare(q) == .orderedSame }) { return a }
    if let a = apps.first(where: { $0.localizedName?.localizedCaseInsensitiveContains(q) == true }) { return a }
    fail("app not running: \(q)  (try: axhelper apps)", 4)
}

/// Windows (+ menu bar with --menus). Activates the app first: several apps (KakaoTalk…)
/// only expose their windows to Accessibility while frontmost. If the wanted window is
/// missing (e.g. the main window was closed) the app is asked to reopen once.
func roots(_ ra: NSRunningApplication) -> [Node] {
    if !AXIsProcessTrusted() { fail("Accessibility permission missing for this process (System Settings › Privacy › Accessibility)", 9) }
    let app = axApp(ra)
    if !flags.contains("--no-activate") && !ra.isActive {
        ra.activate()
        usleep(150_000)
    }
    let want = opts["--window"]
    var wins: [AXUIElement] = []
    var reopened = false
    for i in 0..<30 {  // up to ~3s
        wins = pickWindows(app, want)
        if !wins.isEmpty { break }
        if i == 8 && !reopened, let id = ra.bundleIdentifier {
            NSAppleScript(source: "tell application id \"\(id)\" to reopen")?.executeAndReturnError(nil)
            reopened = true
        }
        usleep(100_000)
    }
    var out = wins.enumerated().map { Node($0.element, "\($0.offset)", 0) }
    if flags.contains("--menus"), let mb = attr(app, kAXMenuBarAttribute) {
        out.append(Node(mb as! AXUIElement, "m", 0))
    }
    if out.isEmpty {
        fail(want.map { "no window titled \($0)" } ?? "\(ra.localizedName ?? "app") exposes no windows (is it frontmost / not minimised?)", 3)
    }
    return out
}

func axApp(_ ra: NSRunningApplication) -> AXUIElement {
    let app = AXUIElementCreateApplication(ra.processIdentifier)
    AXUIElementSetMessagingTimeout(app, 3.0)
    return app
}

func windowTitle(_ w: AXUIElement) -> String { str(attr(w, kAXTitleAttribute)) ?? "" }

/// exact title match wins; otherwise "contains"
func pickWindows(_ app: AXUIElement, _ want: String?) -> [AXUIElement] {
    let wins = (attr(app, kAXWindowsAttribute) as? [AXUIElement]) ?? []
    guard let t = want else { return wins }
    let exact = wins.filter { windowTitle($0) == t }
    return exact.isEmpty ? wins.filter { windowTitle($0).localizedCaseInsensitiveContains(t) } : exact
}

func walk(_ roots: [Node], _ visit: (Node) -> Bool) {  // DFS; visit returns false to stop
    var stack = Array(roots.reversed())
    while let n = stack.popLast() {
        if !visit(n) { return }
        if n.depth >= maxDepth { continue }
        for (i, k) in n.kids.enumerated().reversed() { stack.append(Node(k, "\(n.path).\(i)", n.depth + 1)) }
    }
}

// ---------------------------------------------------------------- query

struct Query {
    var terms: [(key: String, op: Character, val: String)] = []
    var nth = 1
    var path: String?

    init(_ s: String) {
        for t in Query.tokens(s) {
            if t.hasPrefix("#"), let n = Int(t.dropFirst()) { nth = max(1, n); continue }
            if t.hasPrefix("path=") { path = String(t.dropFirst(5)); continue }
            if let i = t.firstIndex(where: { $0 == "=" || $0 == "~" }),
               ["role", "title", "desc", "value"].contains(String(t[..<i])) {
                terms.append((String(t[..<i]), t[i], String(t[t.index(after: i)...])))
            } else {
                terms.append(("label", "~", t))
            }
        }
    }

    /// split on spaces; double quotes group (desc="메시지 입력") and are dropped
    static func tokens(_ s: String) -> [String] {
        var out: [String] = [], cur = "", quoted = false
        for c in s {
            if c == "\"" { quoted.toggle(); continue }
            if c == " " && !quoted {
                if !cur.isEmpty { out.append(cur); cur = "" }
            } else {
                cur.append(c)
            }
        }
        if !cur.isEmpty { out.append(cur) }
        return out
    }

    func matches(_ n: Node) -> Bool {
        terms.allSatisfy { t in
            let v: String
            switch t.key {
            case "role": v = n.role
            case "title": v = n.title
            case "desc": v = n.desc
            case "value": v = n.value
            default: v = n.label
            }
            return t.op == "=" ? v == t.val : v.localizedCaseInsensitiveContains(t.val)
        }
    }
}

func findAll(_ rs: [Node], _ q: Query, limit: Int) -> [Node] {
    var hits: [Node] = []
    walk(rs) { n in
        if let p = q.path {
            if n.path == p { hits.append(n); return false }
            return true
        }
        if !q.terms.isEmpty && q.matches(n) { hits.append(n) }
        return hits.count < limit
    }
    return hits
}

func target(_ rs: [Node], _ qs: String) -> Node {
    let q = Query(qs)
    let hits = findAll(rs, q, limit: q.nth)
    guard hits.count >= q.nth else { fail("no element matches: \(qs)", 5) }
    return hits[q.nth - 1]
}

// ---------------------------------------------------------------- actions

func click(_ n: Node) -> Bool {
    guard let p = attr(n.el, kAXPositionAttribute), let s = attr(n.el, kAXSizeAttribute) else { return false }
    var pt = CGPoint.zero, sz = CGSize.zero
    AXValueGetValue(p as! AXValue, .cgPoint, &pt)
    AXValueGetValue(s as! AXValue, .cgSize, &sz)
    let c = CGPoint(x: pt.x + sz.width / 2, y: pt.y + sz.height / 2)
    for t in [CGEventType.leftMouseDown, .leftMouseUp] {
        CGEvent(mouseEventSource: nil, mouseType: t, mouseCursorPosition: c, mouseButton: .left)?.post(tap: .cghidEventTap)
        usleep(30_000)
    }
    return true
}

func postKey(_ pid: pid_t, _ name: String) -> Bool {
    let codes: [String: CGKeyCode] = ["return": 36, "enter": 36, "tab": 48, "space": 49, "delete": 51,
                                      "escape": 53, "esc": 53, "left": 123, "right": 124, "down": 125, "up": 126]
    guard let code = codes[name.lowercased()] else { return false }
    let src = CGEventSource(stateID: .hidSystemState)
    for down in [true, false] {
        CGEvent(keyboardEventSource: src, virtualKey: code, keyDown: down)?.postToPid(pid)
        usleep(20_000)
    }
    return true
}

func firstText(_ row: AXUIElement) -> String? {
    var stack = [row]
    var seen = 0
    while let el = stack.popLast(), seen < 40 {
        seen += 1
        if str(attr(el, kAXRoleAttribute)) == "AXStaticText" { return str(attr(el, kAXValueAttribute)) }
        stack.append(contentsOf: ((attr(el, kAXChildrenAttribute) as? [AXUIElement]) ?? []).reversed())
    }
    return nil
}

func emit(_ d: [String: Any]) {
    if let data = try? JSONSerialization.data(withJSONObject: d), let s = String(data: data, encoding: .utf8) { print(s) }
}

func runOverlay(_ message: String, _ maxSeconds: Double) -> Never {
    let parent = getppid()
    let accent = NSColor(calibratedRed: 1.0, green: 0.45, blue: 0.0, alpha: 1.0)
    let app = NSApplication.shared
    app.setActivationPolicy(.accessory)  // no Dock icon, never activates

    func overlayWindow(_ frame: NSRect) -> NSWindow {
        let w = NSWindow(contentRect: frame, styleMask: .borderless, backing: .buffered, defer: false)
        w.isOpaque = false
        w.backgroundColor = .clear
        w.level = .screenSaver
        w.ignoresMouseEvents = true
        w.hasShadow = false
        w.collectionBehavior = [.canJoinAllSpaces, .stationary, .ignoresCycle, .fullScreenAuxiliary]
        return w
    }
    final class Border: NSView {
        let color: NSColor
        init(frame: NSRect, color: NSColor) { self.color = color; super.init(frame: frame) }
        required init?(coder: NSCoder) { fatalError() }
        override func draw(_ dirtyRect: NSRect) {
            let p = NSBezierPath(rect: bounds.insetBy(dx: 4, dy: 4))
            p.lineWidth = 8
            color.withAlphaComponent(0.9).setStroke()
            p.stroke()
        }
    }

    var windows: [NSWindow] = []
    for screen in NSScreen.screens {
        let f = screen.frame
        let border = overlayWindow(f)
        border.contentView = Border(frame: NSRect(origin: .zero, size: f.size), color: accent)
        border.orderFrontRegardless()
        windows.append(border)

        let text = "●  \(message)  —  마우스·키보드를 건드리지 마세요"
        let font = NSFont.systemFont(ofSize: 15, weight: .semibold)
        let width = min(f.width - 40, (text as NSString).size(withAttributes: [.font: font]).width + 48)
        let height: CGFloat = 38
        let top = f.maxY - (screen.frame.maxY - screen.visibleFrame.maxY) - height - 10
        let banner = overlayWindow(NSRect(x: f.midX - width / 2, y: top, width: width, height: height))
        let box = NSView(frame: NSRect(x: 0, y: 0, width: width, height: height))
        box.wantsLayer = true
        box.layer?.backgroundColor = accent.cgColor
        box.layer?.borderColor = NSColor.white.cgColor
        box.layer?.borderWidth = 2
        box.layer?.cornerRadius = height / 2
        let label = NSTextField(labelWithString: text)
        label.font = font
        label.textColor = .white
        label.alignment = .center
        label.frame = NSRect(x: 0, y: (height - 20) / 2, width: width, height: 20)
        box.addSubview(label)
        banner.contentView = box
        banner.orderFrontRegardless()
        windows.append(banner)

        Timer.scheduledTimer(withTimeInterval: 0.8, repeats: true) { _ in  // blinking border
            NSAnimationContext.runAnimationGroup { ctx in
                ctx.duration = 0.4
                border.animator().alphaValue = border.alphaValue < 1 ? 1 : 0.35
            }
        }
    }
    let started = Date()
    Timer.scheduledTimer(withTimeInterval: 0.5, repeats: true) { _ in  // safety: parent gone / timeout
        if getppid() != parent || Date().timeIntervalSince(started) > maxSeconds { exit(0) }
    }
    signal(SIGTERM) { _ in exit(0) }
    signal(SIGINT) { _ in exit(0) }
    app.run()
    exit(0)
}

func collectText(_ rs: [Node]) -> [String] {
    var out: [String] = []
    var seen = Set<String>()
    walk(rs) { n in
        for s in [n.title, n.value, n.desc] where !s.isEmpty && !seen.contains(n.path + s) {
            seen.insert(n.path + s)
            out.append(s)
            break  // one label per node keeps it readable
        }
        return true
    }
    return out
}

// ---------------------------------------------------------------- main

guard let cmd = positional.first else { fail("usage: axhelper apps|tree|find|text|press|set|dump|windows|pick|key|overlay …", 2) }
let args = Array(positional.dropFirst())

switch cmd {
case "apps":
    for a in NSWorkspace.shared.runningApplications where a.activationPolicy == .regular {
        print("\(a.localizedName ?? "?") · \(a.bundleIdentifier ?? "-") · pid \(a.processIdentifier)\(a.isActive ? " · active" : "")")
    }

case "tree":
    guard args.count >= 1 else { fail("usage: axhelper tree <app>", 2) }
    var printed = 0
    walk(roots(resolveApp(args[0]))) { n in
        print(String(repeating: " ", count: n.depth) + n.line)
        printed += 1
        if printed >= maxPrint { print("… (truncated at --max \(maxPrint))"); return false }
        return true
    }

case "find":
    guard args.count >= 2 else { fail("usage: axhelper find <app> <query>", 2) }
    let hits = findAll(roots(resolveApp(args[0])), Query(args[1]), limit: maxPrint)
    if hits.isEmpty { fail("no element matches: \(args[1])", 5) }
    for h in hits { print(h.line + h.frame) }

case "text":
    guard args.count >= 1 else { fail("usage: axhelper text <app> [query]", 2) }
    let rs = roots(resolveApp(args[0]))
    print(collectText(args.count >= 2 ? [target(rs, args[1])] : rs).joined(separator: "\n"))

case "press":
    guard args.count >= 2 else { fail("usage: axhelper press <app> <query>", 2) }
    let n = target(roots(resolveApp(args[0])), args[1])
    if AXUIElementPerformAction(n.el, kAXPressAction as CFString) == .success {
        print("pressed \(n.line)")
    } else if click(n) {
        print("clicked \(n.line)\(n.frame)")
    } else {
        fail("can't press or click \(n.line)", 6)
    }

case "set":
    guard args.count >= 3 else { fail("usage: axhelper set <app> <query> <value>", 2) }
    let n = target(roots(resolveApp(args[0])), args[1])
    AXUIElementSetAttributeValue(n.el, kAXFocusedAttribute as CFString, kCFBooleanTrue)
    AXUIElementSetAttributeValue(n.el, kAXValueAttribute as CFString, args[2] as CFString)
    usleep(50_000)
    let got = str(attr(n.el, kAXValueAttribute)) ?? ""
    if got != args[2] { fail("value not applied to \(n.line) (read back: \(got.prefix(60)))", 8) }
    print("set \(n.path) \(n.role) = \(args[2].count) chars")

case "dump":
    guard args.count >= 1 else { fail("usage: axhelper dump <app> [--window t] [--rows N] [--tail]", 2) }
    walk(roots(resolveApp(args[0]))) { n in
        var d: [String: Any] = ["d": n.depth, "r": n.role]
        if !n.title.isEmpty { d["t"] = n.title }
        if !n.value.isEmpty { d["v"] = n.value }
        if !n.desc.isEmpty { d["desc"] = n.desc }
        emit(d)
        return true
    }

case "windows":
    guard args.count >= 1 else { fail("usage: axhelper windows <app>", 2) }
    let ra = resolveApp(args[0])
    if !flags.contains("--no-activate") { ra.activate(); usleep(150_000) }
    for w in pickWindows(axApp(ra), nil) { print(windowTitle(w)) }

case "pick":
    guard args.count >= 2 else { fail("usage: axhelper pick <app> <text> [--window list-window] [--rows N]", 2) }
    let ra = resolveApp(args[0])
    let app = axApp(ra)
    let name = args[1]
    if !pickWindows(app, nil).filter({ windowTitle($0) == name }).isEmpty {
        ra.activate(); print("already-open"); exit(0)
    }
    var table: AXUIElement?
    walk(roots(ra)) { n in
        if n.role == "AXTable" || n.role == "AXOutline" { table = n.el; return false }
        return true
    }
    guard let t = table else { fail("no table/list found", 5) }
    let rows = (attr(t, kAXRowsAttribute) as? [AXUIElement]) ?? []
    let limit = min(rows.count, Int(opts["--rows"] ?? "") ?? 200)
    guard let row = rows.prefix(limit).first(where: { firstText($0) == name }) else {
        fail("no row titled \(name) in the first \(limit) rows", 5)
    }
    AXUIElementSetAttributeValue(t, kAXSelectedRowsAttribute as CFString, [row] as CFArray)
    usleep(80_000)
    _ = postKey(ra.processIdentifier, "return")
    let deadline = Date().addingTimeInterval(3)
    while Date() < deadline {
        if !pickWindows(app, nil).filter({ windowTitle($0) == name }).isEmpty { print("opened"); exit(0) }
        usleep(100_000)
    }
    print("selected")  // row selected + Return sent, but no window titled like it (list-style app)

case "key":
    guard args.count >= 2 else { fail("usage: axhelper key <app> return|escape|tab|…", 2) }
    let ra = resolveApp(args[0])
    if !flags.contains("--no-activate") { ra.activate(); usleep(80_000) }
    if !postKey(ra.processIdentifier, args[1]) { fail("unknown key: \(args[1])", 2) }
    print("ok")

case "overlay":
    runOverlay(args.first ?? "Claude가 컴퓨터를 조작하고 있습니다", Double(args.count >= 2 ? args[1] : "") ?? 120)

default:
    fail("unknown command: \(cmd)", 2)
}
