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
//   axhelper close <app> --window <title>      press a window's close button
//   axhelper snapshot                          all windows (id, pid, owner, frame) — for the window stack
//   axhelper closewin <pid> <x> <y> <w> <h>    close that window (close button, else Cancel)
//   axhelper rowcount <app> [opts]             number of rows in the first table/list
//   axhelper scroll <app> [query] <0..1>       set a scroll bar (default: the first) — 0 top, 1 bottom
//   axhelper locate <app> <query> [opts]       JSON: target path/frame/label + same-role candidates' frames
//   axhelper overlay "<message>" [seconds=120] on-screen border + banner + virtual cursor layer driven
//                                              by JSON lines on stdin (click-through, no focus; exits on
//                                              SIGTERM / parent death)
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
    // regular apps + menu-bar (accessory) apps; background daemons have no UI to drive
    let apps = NSWorkspace.shared.runningApplications.filter { $0.activationPolicy != .prohibited }
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
    // open pop-up / context menus hang off the app, not a window — include them ("p0", "p1"…)
    let popups = ((attr(app, kAXChildrenAttribute) as? [AXUIElement]) ?? []).filter { str(attr($0, kAXRoleAttribute)) == "AXMenu" }
    out.append(contentsOf: popups.enumerated().map { Node($0.element, "p\($0.offset)", 0) })
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
    // move first: some apps ignore a down/up that arrives without the pointer being there
    CGEvent(mouseEventSource: nil, mouseType: .mouseMoved, mouseCursorPosition: c, mouseButton: .left)?.post(tap: .cghidEventTap)
    usleep(60_000)
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

// ---------------------------------------------------------------- overlay (visual layer)
//
// Full-screen, click-through, never-focused windows on every display: blinking orange border,
// top banner, and a visual layer driven by JSON lines on stdin (coordinates are AX/global,
// top-left origin — the same space `locate` and [CLICK] x y use):
//   {"c":"look","cands":[[x,y,w,h],…],"target":[x,y,w,h],"label":"등호"}   candidates + chosen one
//   {"c":"move","x":…,"y":…,"ms":450}                                     glide the virtual cursor
//   {"c":"click","x":…,"y":…}                                             click ripple
//   {"c":"caption","text":"⌨ 입력 중…"}                                     bubble next to the cursor
//   {"c":"clear"}

let accent = NSColor(calibratedRed: 1.0, green: 0.45, blue: 0.0, alpha: 1.0)

final class Visual {
    static let shared = Visual()
    var cursor = NSEvent.mouseLocation          // AppKit global (bottom-left origin)
    var from = NSEvent.mouseLocation, to = NSEvent.mouseLocation
    var moveStart = Date.distantPast, moveDur = 0.45
    var cands: [NSRect] = []
    var target: NSRect?
    var label = ""
    var lookAt = Date.distantPast
    var ripples: [(NSPoint, Date)] = []
    var caption = ""
    var captionAt = Date.distantPast
    var borderOn = true

    /// AX/global top-left → AppKit global bottom-left (relative to the primary screen)
    static func flip(_ x: CGFloat, _ y: CGFloat) -> NSPoint {
        NSPoint(x: x, y: (NSScreen.screens.first?.frame.maxY ?? 0) - y)
    }
    static func flipRect(_ a: [Double]) -> NSRect? {
        guard a.count >= 4 else { return nil }
        let o = flip(a[0], a[1] + a[3])
        return NSRect(x: o.x, y: o.y, width: a[2], height: a[3])
    }

    func tick() {
        let t = min(1, Date().timeIntervalSince(moveStart) / moveDur)
        let e = t < 0.5 ? 4 * t * t * t : 1 - pow(-2 * t + 2, 3) / 2  // easeInOutCubic
        cursor = NSPoint(x: from.x + (to.x - from.x) * e, y: from.y + (to.y - from.y) * e)
        ripples.removeAll { Date().timeIntervalSince($0.1) > 0.7 }
    }

    func apply(_ m: [String: Any]) {
        let num = { (k: String) in (m[k] as? NSNumber)?.doubleValue ?? 0 }
        switch m["c"] as? String ?? "" {
        case "look":
            cands = ((m["cands"] as? [[Double]]) ?? []).compactMap(Visual.flipRect)
            target = (m["target"] as? [Double]).flatMap(Visual.flipRect)
            label = m["label"] as? String ?? ""
            lookAt = Date()
        case "move":
            from = cursor
            to = Visual.flip(num("x"), num("y"))
            moveDur = max(0.05, num("ms") / 1000)
            moveStart = Date()
        case "click":
            ripples.append((Visual.flip(num("x"), num("y")), Date()))
        case "caption":
            caption = m["text"] as? String ?? ""
            captionAt = Date()
        case "clear":
            cands = []; target = nil; caption = ""
        default: break
        }
    }
}

final class FXView: NSView {
    override var isFlipped: Bool { false }

    func local(_ p: NSPoint) -> NSPoint {
        guard let w = window else { return p }
        return NSPoint(x: p.x - w.frame.minX, y: p.y - w.frame.minY)
    }
    func local(_ r: NSRect) -> NSRect { NSRect(origin: local(r.origin), size: r.size) }

    override func draw(_ dirty: NSRect) {
        let v = Visual.shared
        // border
        let b = NSBezierPath(rect: bounds.insetBy(dx: 4, dy: 4))
        b.lineWidth = 8
        accent.withAlphaComponent(v.borderOn ? 0.9 : 0.35).setStroke()
        b.stroke()

        // candidates the agent is considering, fading after a few seconds
        let lookAge = Date().timeIntervalSince(v.lookAt)
        let lookAlpha = CGFloat(max(0, min(1, (4.0 - lookAge) / 1.0)))
        if lookAlpha > 0 {
            for c in v.cands {
                let r = NSBezierPath(roundedRect: local(c).insetBy(dx: -2, dy: -2), xRadius: 6, yRadius: 6)
                r.lineWidth = 1.5
                r.setLineDash([5, 4], count: 2, phase: 0)
                NSColor.systemBlue.withAlphaComponent(0.55 * lookAlpha).setStroke()
                r.stroke()
            }
            if let t = v.target {
                let rr = local(t).insetBy(dx: -4, dy: -4)
                let r = NSBezierPath(roundedRect: rr, xRadius: 8, yRadius: 8)
                accent.withAlphaComponent(0.18 * lookAlpha).setFill()
                r.fill()
                r.lineWidth = 3
                accent.withAlphaComponent(lookAlpha).setStroke()
                r.stroke()
                if !v.label.isEmpty { pill("🎯 " + v.label, at: NSPoint(x: rr.minX, y: rr.maxY + 6), alpha: lookAlpha) }
            }
        }

        // click ripples
        for (p, t) in v.ripples {
            let age = CGFloat(Date().timeIntervalSince(t) / 0.7)
            let c = local(p)
            let rad = 8 + 34 * age
            let o = NSBezierPath(ovalIn: NSRect(x: c.x - rad, y: c.y - rad, width: rad * 2, height: rad * 2))
            o.lineWidth = 3
            accent.withAlphaComponent(1 - age).setStroke()
            o.stroke()
        }

        // virtual cursor (arrow) + name tag + caption
        let c = local(v.cursor)
        guard bounds.insetBy(dx: -40, dy: -40).contains(c) else { return }
        let a = NSBezierPath()
        a.move(to: c)
        a.line(to: NSPoint(x: c.x, y: c.y - 26))
        a.line(to: NSPoint(x: c.x + 7, y: c.y - 20))
        a.line(to: NSPoint(x: c.x + 12, y: c.y - 31))
        a.line(to: NSPoint(x: c.x + 17, y: c.y - 29))
        a.line(to: NSPoint(x: c.x + 12, y: c.y - 18))
        a.line(to: NSPoint(x: c.x + 20, y: c.y - 18))
        a.close()
        NSGraphicsContext.saveGraphicsState()
        let sh = NSShadow()
        sh.shadowColor = NSColor.black.withAlphaComponent(0.4)
        sh.shadowBlurRadius = 4
        sh.shadowOffset = NSSize(width: 1, height: -2)
        sh.set()
        accent.setFill()
        a.fill()
        NSGraphicsContext.restoreGraphicsState()
        NSColor.white.setStroke()
        a.lineWidth = 2
        a.stroke()
        pill("Claude", at: NSPoint(x: c.x + 18, y: c.y - 46), alpha: 1, small: true)
        if !v.caption.isEmpty && Date().timeIntervalSince(v.captionAt) < 4 {
            pill(v.caption, at: NSPoint(x: c.x + 18, y: c.y - 74), alpha: 1)
        }
    }

    func pill(_ text: String, at p: NSPoint, alpha: CGFloat, small: Bool = false) {
        let font = NSFont.systemFont(ofSize: small ? 11 : 13, weight: .semibold)
        let attrs: [NSAttributedString.Key: Any] = [.font: font, .foregroundColor: NSColor.white.withAlphaComponent(alpha)]
        let sz = (text as NSString).size(withAttributes: attrs)
        let r = NSRect(x: p.x, y: p.y, width: sz.width + 16, height: sz.height + 6)
        accent.withAlphaComponent(alpha).setFill()
        NSBezierPath(roundedRect: r, xRadius: r.height / 2, yRadius: r.height / 2).fill()
        (text as NSString).draw(at: NSPoint(x: r.minX + 8, y: r.minY + 3), withAttributes: attrs)
    }
}

func runOverlay(_ message: String, _ maxSeconds: Double) -> Never {
    let parent = getppid()
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

    var views: [FXView] = []
    var windows: [NSWindow] = []
    for screen in NSScreen.screens {
        let f = screen.frame
        let fx = overlayWindow(f)
        let v = FXView(frame: NSRect(origin: .zero, size: f.size))
        fx.contentView = v
        fx.orderFrontRegardless()
        views.append(v)
        windows.append(fx)

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
    }

    // commands on stdin
    FileHandle.standardInput.readabilityHandler = { h in
        let d = h.availableData
        if d.isEmpty { h.readabilityHandler = nil; return }
        for line in d.split(separator: 0x0A) {
            if let m = try? JSONSerialization.jsonObject(with: Data(line)) as? [String: Any] {
                DispatchQueue.main.async { Visual.shared.apply(m) }
            }
        }
    }
    Timer.scheduledTimer(withTimeInterval: 1.0 / 60, repeats: true) { _ in  // animation
        Visual.shared.tick()
        views.forEach { $0.needsDisplay = true }
    }
    Timer.scheduledTimer(withTimeInterval: 0.8, repeats: true) { _ in Visual.shared.borderOn.toggle() }
    let started = Date()
    Timer.scheduledTimer(withTimeInterval: 0.5, repeats: true) { _ in  // safety: parent gone / timeout
        if getppid() != parent || Date().timeIntervalSince(started) > maxSeconds { exit(0) }
    }
    signal(SIGTERM) { _ in exit(0) }
    signal(SIGINT) { _ in exit(0) }
    _ = windows
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

guard let cmd = positional.first else { fail("usage: axhelper apps|tree|find|text|locate|close|snapshot|closewin|rowcount|scroll|press|set|dump|windows|pick|key|overlay …", 2) }
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

case "locate":
    // target + same-role candidates in the same window, with frames — for the visual layer
    guard args.count >= 2 else { fail("usage: axhelper locate <app> <query>", 2) }
    let rs = roots(resolveApp(args[0]))
    let t = target(rs, args[1])
    func rect(_ n: Node) -> [Int]? {
        guard let p = attr(n.el, kAXPositionAttribute), let s = attr(n.el, kAXSizeAttribute) else { return nil }
        var pt = CGPoint.zero, sz = CGSize.zero
        AXValueGetValue(p as! AXValue, .cgPoint, &pt)
        AXValueGetValue(s as! AXValue, .cgSize, &sz)
        return [Int(pt.x), Int(pt.y), Int(sz.width), Int(sz.height)]
    }
    let win = t.path.split(separator: ".").first.map(String.init) ?? ""
    var cands: [[Int]] = []
    walk(rs.filter { $0.path == win }) { n in
        if n.role == t.role, n.path != t.path, let r = rect(n), r[2] > 0, r[3] > 0 { cands.append(r) }
        return cands.count < 60
    }
    var out: [String: Any] = ["path": t.path, "role": t.role, "label": t.label, "cands": cands]
    if let r = rect(t) { out["frame"] = r }
    emit(out)

case "snapshot":
    // ALL normal windows (layer 0) — every Space, minimised, hidden — front to back:
    // {"id","pid","owner","b":[x,y,w,h]}. Not on-screen-only: switching Spaces would make
    // old windows look new.
    let list = (CGWindowListCopyWindowInfo([.optionAll, .excludeDesktopElements], kCGNullWindowID) as? [[String: Any]]) ?? []
    // only regular apps' windows — system helpers (CursorUIViewService…) are not ours to close
    let regular = Set(NSWorkspace.shared.runningApplications.filter { $0.activationPolicy == .regular }.map { Int($0.processIdentifier) })
    for w in list where (w[kCGWindowLayer as String] as? Int ?? -1) == 0 && regular.contains(w[kCGWindowOwnerPID as String] as? Int ?? -1) {
        let b = w[kCGWindowBounds as String] as? [String: Any] ?? [:]
        let r = ["X", "Y", "Width", "Height"].map { Int((b[$0] as? NSNumber)?.doubleValue ?? 0) }
        emit(["id": w[kCGWindowNumber as String] as? Int ?? 0, "pid": w[kCGWindowOwnerPID as String] as? Int ?? 0,
              "owner": w[kCGWindowOwnerName as String] as? String ?? "", "b": r])
    }

case "closewin":
    // close the window of <pid> at frame x y w h (from snapshot): close button, else Cancel
    guard args.count >= 5, let pid = Int32(args[0]) else { fail("usage: axhelper closewin <pid> <x> <y> <w> <h>", 2) }
    let want = args[1...4].compactMap { Int($0) }
    guard want.count == 4 else { fail("bad frame", 2) }
    let app = AXUIElementCreateApplication(pid)
    AXUIElementSetMessagingTimeout(app, 3.0)
    func frameOf(_ el: AXUIElement) -> [Int] {
        guard let p = attr(el, kAXPositionAttribute), let s = attr(el, kAXSizeAttribute) else { return [] }
        var pt = CGPoint.zero, sz = CGSize.zero
        AXValueGetValue(p as! AXValue, .cgPoint, &pt)
        AXValueGetValue(s as! AXValue, .cgSize, &sz)
        return [Int(pt.x), Int(pt.y), Int(sz.width), Int(sz.height)]
    }
    let wins = (attr(app, kAXWindowsAttribute) as? [AXUIElement]) ?? []
    guard let w = wins.first(where: { f in let r = frameOf(f); return r.count == 4 && zip(r, want).allSatisfy { abs($0 - $1) <= 2 } }) else {
        fail("no window of pid \(pid) at \(want)", 5)
    }
    for key in [kAXCloseButtonAttribute, kAXCancelButtonAttribute] {
        if let b = attr(w, key) {
            if AXUIElementPerformAction(b as! AXUIElement, kAXPressAction as CFString) == .success {
                print("closed \(windowTitle(w))"); exit(0)
            }
        }
    }
    fail("window \(windowTitle(w)) has no close/cancel button", 6)

case "close":
    // press the window's close button (works where ⌘W doesn't)
    guard args.count >= 1, opts["--window"] != nil else { fail("usage: axhelper close <app> --window <title>", 2) }
    let w = roots(resolveApp(args[0])).first { $0.role == "AXWindow" }
    guard let win = w, let b = attr(win.el, kAXCloseButtonAttribute) else { fail("no close button", 6) }
    if AXUIElementPerformAction(b as! AXUIElement, kAXPressAction as CFString) != .success { fail("close failed", 6) }
    print("closed \(windowTitle(win.el))")

case "rowcount":
    // rows in the first table/list (e.g. how much chat history is loaded)
    guard args.count >= 1 else { fail("usage: axhelper rowcount <app> [--window t]", 2) }
    var table: AXUIElement?
    walk(roots(resolveApp(args[0]))) { n in
        if n.role == "AXTable" || n.role == "AXOutline" { table = n.el; return false }
        return true
    }
    guard let t = table else { fail("no table/list found", 5) }
    print(((attr(t, kAXRowsAttribute) as? [AXUIElement]) ?? []).count)

case "scroll":
    // set the first scroll bar (or the matched one) to a position: 0 = top, 1 = bottom
    guard args.count >= 2, let pos = Double(args.last!) else { fail("usage: axhelper scroll <app> [query] <0..1>", 2) }
    let rs = roots(resolveApp(args[0]))
    let bar = args.count >= 3 ? target(rs, args[1]) : target(rs, "role=AXScrollBar")
    let r = AXUIElementSetAttributeValue(bar.el, kAXValueAttribute as CFString, NSNumber(value: pos))
    if r != .success { fail("can't scroll \(bar.line) (AX error \(r.rawValue))", 8) }
    print("scrolled \(bar.path) to \(pos)")

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
    if let w = pickWindows(app, nil).first(where: { windowTitle($0) == name }) {
        ra.activate()
        AXUIElementPerformAction(w, kAXRaiseAction as CFString)  // bring that window to the front
        AXUIElementSetAttributeValue(w, kAXMainAttribute as CFString, kCFBooleanTrue)
        print("already-open"); exit(0)
    }
    var table: AXUIElement?
    var listWin: AXUIElement?
    let rs = roots(ra)
    walk(rs) { n in
        if n.role == "AXTable" || n.role == "AXOutline" {
            table = n.el
            listWin = rs.first { $0.path == String(n.path.split(separator: ".").first ?? "") }?.el
            return false
        }
        return true
    }
    guard let t = table else { fail("no table/list found", 5) }
    if let w = listWin {  // Return goes to the key window — make it the list's window
        AXUIElementPerformAction(w, kAXRaiseAction as CFString)
        AXUIElementSetAttributeValue(w, kAXMainAttribute as CFString, kCFBooleanTrue)
        usleep(80_000)
    }
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
