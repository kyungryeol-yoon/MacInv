// 앱 아이콘(F1: 목록 카드 — 앱 타일 3줄 + 용량 막대 + 상태 배지 ↑ ✓ −)을 그려 packaging/AppIcon.icns 를 만듦
//   swiftc -o /tmp/make-icon packaging/make-icon.swift -framework AppKit && /tmp/make-icon packaging
// SF Symbol은 쓰지 않는다(애플 라이선스상 앱 아이콘·로고 사용 금지) — 배지 기호도 도형으로 그림
import AppKit

let c = { (r: CGFloat, g: CGFloat, b: CGFloat) in NSColor(srgbRed: r/255, green: g/255, blue: b/255, alpha: 1) }
var S: CGFloat = 1
func P(_ x: CGFloat, _ y: CGFloat) -> NSPoint { NSPoint(x: x * S, y: y * S) }
func R(_ x: CGFloat, _ y: CGFloat, _ w: CGFloat, _ h: CGFloat) -> NSRect { NSRect(x: x * S, y: y * S, width: w * S, height: h * S) }

func shadow(_ a: CGFloat, _ blur: CGFloat, _ dy: CGFloat, _ body: () -> Void) {
  NSGraphicsContext.saveGraphicsState()
  let sh = NSShadow(); sh.shadowColor = .black.withAlphaComponent(a); sh.shadowBlurRadius = blur * S; sh.shadowOffset = NSSize(width: 0, height: -dy * S); sh.set()
  body()
  NSGraphicsContext.restoreGraphicsState()
}
func fill(_ p: NSBezierPath, _ top: NSColor, _ bottom: NSColor, angle: CGFloat = -90) { NSGradient(starting: top, ending: bottom)!.draw(in: p, angle: angle) }
func poly(_ pts: [(CGFloat, CGFloat)]) -> NSBezierPath {
  let p = NSBezierPath(); p.move(to: P(pts[0].0, pts[0].1)); for q in pts.dropFirst() { p.line(to: P(q.0, q.1)) }; p.close(); p.lineJoinStyle = .round; return p
}

/// 바탕: DiSwitch와 같은 824 둥근 사각형, 어두운 그라데이션
func background() {
  let base = NSBezierPath(roundedRect: R(100, 100, 824, 824), xRadius: 185.4 * S, yRadius: 185.4 * S)
  shadow(0.28, 20, 8) { c(58, 60, 66).setFill(); base.fill() }
  NSGradient(starting: c(58, 60, 66), ending: c(24, 25, 28))!.draw(in: base, angle: -90)
}

func tile(_ x: CGFloat, _ y: CGFloat, _ s: CGFloat, _ col: (NSColor, NSColor)) {
  let p = NSBezierPath(roundedRect: R(x, y, s, s), xRadius: s * 0.24 * S, yRadius: s * 0.24 * S)
  shadow(0.35, 10, 4) { col.0.setFill(); p.fill() }
  fill(p, col.0, col.1)
}

enum Mark { case up, check, minus }
/// 상태 배지: ↑ 업데이트(파랑) · ✓ 최신(초록) · − 삭제(빨강)
func badge(_ cx: CGFloat, _ cy: CGFloat, _ r: CGFloat, _ m: Mark) {
  let col: (NSColor, NSColor) = m == .up ? (c(70, 160, 255), c(20, 100, 235)) : m == .check ? (c(80, 215, 120), c(36, 170, 80)) : (c(255, 100, 90), c(225, 50, 50))
  let o = NSBezierPath(ovalIn: R(cx - r, cy - r, 2 * r, 2 * r))
  shadow(0.35, 10, 3) { col.0.setFill(); o.fill() }
  fill(o, col.0, col.1)
  let p = NSBezierPath(); p.lineWidth = r * 0.26 * S; p.lineCapStyle = .round; p.lineJoinStyle = .round
  let k = r * 0.48
  switch m {
  case .up: p.move(to: P(cx, cy - k)); p.line(to: P(cx, cy + k)); p.move(to: P(cx - k * 0.75, cy + k * 0.2)); p.line(to: P(cx, cy + k)); p.line(to: P(cx + k * 0.75, cy + k * 0.2))
  case .check: p.move(to: P(cx - k * 0.8, cy)); p.line(to: P(cx - k * 0.2, cy - k * 0.6)); p.line(to: P(cx + k * 0.85, cy + k * 0.6))
  case .minus: p.move(to: P(cx - k * 0.8, cy)); p.line(to: P(cx + k * 0.8, cy))
  }
  NSColor.white.setStroke(); p.stroke()
}

/// 화면 안 목록: 앱 타일(주황·파랑·보라) + 용량 막대 + 오른쪽 배지
func listContent(_ x: CGFloat, _ y: CGFloat, _ w: CGFloat, _ h: CGFloat) {
  let cols = [(c(255, 160, 70), c(235, 110, 40)), (c(80, 170, 255), c(30, 100, 230)), (c(190, 120, 255), c(130, 70, 230))]
  let fs: [CGFloat] = [0.9, 0.6, 0.4], marks: [Mark] = [.up, .check, .minus]
  let rowH = h / 3, ts = min(rowH * 0.56, 84), br = ts * 0.62
  for i in 0..<3 {
    let cy = y + h - rowH * (CGFloat(i) + 0.5)
    tile(x + w * 0.07, cy - ts / 2, ts, cols[i])
    let bx = x + w * 0.07 + ts + w * 0.05, bw = (x + w - w * 0.07 - br * 2 - w * 0.05) - bx, bh = ts * 0.36
    c(210, 213, 220).setFill()
    NSBezierPath(roundedRect: R(bx, cy - bh / 2, bw, bh), xRadius: bh / 2 * S, yRadius: bh / 2 * S).fill()
    fill(NSBezierPath(roundedRect: R(bx, cy - bh / 2, bw * fs[i], bh), xRadius: bh / 2 * S, yRadius: bh / 2 * S), c(90, 94, 104), c(60, 63, 72))
    badge(x + w - w * 0.07 - br, cy, br, marks[i])
  }
}

func appIcon() {
  background()
  // 큰 목록 카드(밝은 바탕) — 메뉴 막대 checklist와 같은 "목록 + 줄 끝 상태"
  let card = NSBezierPath(roundedRect: R(190, 220, 644, 584), xRadius: 64 * S, yRadius: 64 * S)
  shadow(0.45, 28, 12) { c(240, 241, 244).setFill(); card.fill() }
  fill(card, c(250, 251, 253), c(222, 226, 233))
  listContent(190, 250, 644, 524)
}

func make(_ size: CGFloat) -> NSImage {
  NSImage(size: NSSize(width: size, height: size), flipped: false) { _ in S = size / 1024; appIcon(); return true }
}

let out = CommandLine.arguments.count > 1 ? CommandLine.arguments[1] : "."
let set = URL(fileURLWithPath: NSTemporaryDirectory()).appendingPathComponent("MacInvAppIcon.iconset")
try? FileManager.default.removeItem(at: set)
try! FileManager.default.createDirectory(at: set, withIntermediateDirectories: true)
for base in [16, 32, 128, 256, 512] {
  for scale in [1, 2] {
    let px = base * scale
    let rep = NSBitmapImageRep(bitmapDataPlanes: nil, pixelsWide: px, pixelsHigh: px, bitsPerSample: 8, samplesPerPixel: 4,
                               hasAlpha: true, isPlanar: false, colorSpaceName: .deviceRGB, bytesPerRow: 0, bitsPerPixel: 0)!
    rep.size = NSSize(width: px, height: px)
    NSGraphicsContext.saveGraphicsState()
    NSGraphicsContext.current = NSGraphicsContext(bitmapImageRep: rep)
    make(CGFloat(px)).draw(in: NSRect(x: 0, y: 0, width: px, height: px))
    NSGraphicsContext.restoreGraphicsState()
    let name = scale == 1 ? "icon_\(base)x\(base).png" : "icon_\(base)x\(base)@2x.png"
    try! rep.representation(using: .png, properties: [:])!.write(to: set.appendingPathComponent(name))
  }
}
let p = Process()
p.executableURL = URL(fileURLWithPath: "/usr/bin/iconutil")
p.arguments = ["-c", "icns", set.path, "-o", "\(out)/AppIcon.icns"]
try! p.run(); p.waitUntilExit()
print(p.terminationStatus == 0 ? "만듦: \(out)/AppIcon.icns" : "iconutil 실패")
