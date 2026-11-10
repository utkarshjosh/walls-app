// Draws Still's PLACEHOLDER icons with CoreGraphics. Replace the artwork by
// editing this file (or dropping a real 1024x1024 PNG at the same path) and
// running `npm run icons` from apps/desktop, which also regenerates the
// platform icon set with `tauri icon`.
//
// Output:
//   src-tauri/icons/source/app-icon.png   1024x1024 app icon source
//   src-tauri/icons/tray-template.png     36x36 menu-bar glyph (black + alpha;
//                                         macOS tints "template" images itself)

import CoreGraphics
import Foundation
import ImageIO
import UniformTypeIdentifiers

let root = URL(fileURLWithPath: FileManager.default.currentDirectoryPath)
let iconsDir = root.appendingPathComponent("src-tauri/icons")

func color(_ hex: UInt32, _ alpha: CGFloat = 1) -> CGColor {
    CGColor(
        srgbRed: CGFloat((hex >> 16) & 0xFF) / 255,
        green: CGFloat((hex >> 8) & 0xFF) / 255,
        blue: CGFloat(hex & 0xFF) / 255,
        alpha: alpha
    )
}

func makeContext(_ size: Int) -> CGContext {
    CGContext(
        data: nil,
        width: size,
        height: size,
        bitsPerComponent: 8,
        bytesPerRow: 0,
        space: CGColorSpace(name: CGColorSpace.sRGB)!,
        bitmapInfo: CGImageAlphaInfo.premultipliedLast.rawValue
    )!
}

func write(_ context: CGContext, to url: URL) {
    try! FileManager.default.createDirectory(
        at: url.deletingLastPathComponent(), withIntermediateDirectories: true)
    let destination = CGImageDestinationCreateWithURL(
        url as CFURL, UTType.png.identifier as CFString, 1, nil)!
    CGImageDestinationAddImage(destination, context.makeImage()!, nil)
    guard CGImageDestinationFinalize(destination) else { fatalError("could not write \(url.path)") }
    print("wrote \(url.path)")
}

// MARK: App icon — a dark tile with a champagne glow rising over a horizon.

func drawAppIcon() {
    let ctx = makeContext(1024)
    let space = CGColorSpace(name: CGColorSpace.sRGB)!
    // Apple's macOS icon grid: an 824pt rounded square centred on a 1024pt canvas.
    let tile = CGRect(x: 100, y: 100, width: 824, height: 824)
    let tilePath = CGPath(roundedRect: tile, cornerWidth: 186, cornerHeight: 186, transform: nil)

    ctx.saveGState()
    ctx.setShadow(offset: CGSize(width: 0, height: -10), blur: 24, color: color(0x000000, 0.35))
    ctx.addPath(tilePath)
    ctx.setFillColor(color(0x151514))
    ctx.fillPath()
    ctx.restoreGState()

    ctx.saveGState()
    ctx.addPath(tilePath)
    ctx.clip()

    let background = CGGradient(
        colorsSpace: space,
        colors: [color(0x1D1D1B), color(0x151514), color(0x10100F)] as CFArray,
        locations: [0, 0.55, 1])!
    ctx.drawLinearGradient(
        background, start: CGPoint(x: 512, y: tile.maxY), end: CGPoint(x: 512, y: tile.minY),
        options: [])

    // The horizon is the top of a very large circle whose centre sits far below the tile.
    let horizonY: CGFloat = 418
    let radius: CGFloat = 1180
    let centre = CGPoint(x: 512, y: horizonY - radius)
    let planet = CGRect(
        x: centre.x - radius, y: centre.y - radius, width: radius * 2, height: radius * 2)

    let glow = CGGradient(
        colorsSpace: space,
        colors: [
            color(0xCBB895, 0.62), color(0xCBB895, 0.22), color(0xCBB895, 0.05), color(0xCBB895, 0),
        ] as CFArray,
        locations: [0, 0.32, 0.68, 1])!
    ctx.drawRadialGradient(
        glow,
        startCenter: CGPoint(x: 512, y: horizonY - 30), startRadius: 0,
        endCenter: CGPoint(x: 512, y: horizonY - 30), endRadius: 470,
        options: [])

    ctx.addEllipse(in: planet)
    ctx.setFillColor(color(0x0D0D0C))
    ctx.fillPath()

    // A soft bloom under a crisp rim so the edge reads at small sizes.
    ctx.setLineCap(.round)
    ctx.addEllipse(in: planet)
    ctx.setStrokeColor(color(0xCBB895, 0.20))
    ctx.setLineWidth(26)
    ctx.strokePath()
    ctx.addEllipse(in: planet)
    ctx.setStrokeColor(color(0xE4D6BA, 0.95))
    ctx.setLineWidth(7)
    ctx.strokePath()
    ctx.restoreGState()

    // Hairline edge so the dark tile keeps its shape on dark Dock backgrounds.
    ctx.addPath(
        CGPath(
            roundedRect: tile.insetBy(dx: 1.5, dy: 1.5), cornerWidth: 184.5, cornerHeight: 184.5,
            transform: nil))
    ctx.setStrokeColor(color(0xFFFFFF, 0.07))
    ctx.setLineWidth(3)
    ctx.strokePath()

    write(ctx, to: iconsDir.appendingPathComponent("source/app-icon.png"))
}

// MARK: Menu-bar glyph — a picture frame holding the same horizon.

func drawTrayIcon() {
    let size = 36
    let ctx = makeContext(size)
    let black = color(0x000000)
    let frame = CGRect(x: 4, y: 6, width: 28, height: 24)
    let framePath = CGPath(roundedRect: frame, cornerWidth: 6.5, cornerHeight: 6.5, transform: nil)

    ctx.saveGState()
    ctx.addPath(framePath)
    ctx.clip()
    let radius: CGFloat = 30
    ctx.addEllipse(
        in: CGRect(x: 18 - radius, y: 17 - radius * 2, width: radius * 2, height: radius * 2))
    ctx.setFillColor(black)
    ctx.fillPath()
    ctx.restoreGState()

    ctx.addPath(
        CGPath(
            roundedRect: frame.insetBy(dx: 1.25, dy: 1.25), cornerWidth: 5.25, cornerHeight: 5.25,
            transform: nil))
    ctx.setStrokeColor(black)
    ctx.setLineWidth(2.5)
    ctx.strokePath()

    write(ctx, to: iconsDir.appendingPathComponent("tray-template.png"))
}

drawAppIcon()
drawTrayIcon()
