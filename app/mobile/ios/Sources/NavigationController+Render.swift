import UIKit
import WebKit

@available(iOS 26.0, *)
extension NavigationController {
  static let welcomeZ = Depth.welcome
  private static let idePosition = Double(Screen.ide.rawValue)
  private static let filesGap = Screen.ide.rawValue

  private func transform(_ offset: Double) -> CGAffineTransform {
    CGAffineTransform(translationX: CGFloat(offset), y: 0)
  }

  func render() {
    guard let host, let webview else { return }
    machine.width = Double(host.bounds.width)
    let frames = machine.frames
    let width = host.bounds.width

    let radius = renderCards(frames, webview: webview)
    renderShadow(frames, radius: radius)

    park(list, by: machine.position < 1 ? 0 : width)
    park(files, by: machine.position > Self.idePosition ? 0 : width)
    list.layer.zPosition = Depth.underlay
    files.layer.zPosition = Depth.underlay

    place(
      chatHeader,
      as: HeaderPlacement(
        offset: frames.chatOffset,
        alpha: frames.chatHeaderAlpha,
        depth: Depth.chatHeader
      )
    )
    place(
      ideHeader,
      as: HeaderPlacement(
        offset: frames.ideOffset,
        alpha: frames.ideHeaderAlpha,
        depth: Depth.ideHeader
      )
    )
    placeCatcher(width: width, height: host.bounds.height)
  }

  private func movingCard(
    _ frames: Frames,
    webview: WKWebView
  ) -> UIView? {
    guard let gap = frames.gap else { return nil }
    return gap == Self.filesGap ? webview : (snapshot ?? webview)
  }

  private func renderCards(
    _ frames: Frames,
    webview: WKWebView
  ) -> CGFloat {
    let webIsIde = machine.hasSnapshot || machine.web >= .ide
    webview.transform = transform(webIsIde ? frames.ideOffset : frames.chatOffset)
    snapshot?.transform = transform(frames.chatOffset)

    let moving = movingCard(frames, webview: webview)
    let radius = Metric.tapTarget * CGFloat(min(1, frames.amount * Motion.rampFactor))
    for card in [webview, snapshot].compactMap({ $0 }) {
      let rounded = card === moving
      card.layer.cornerRadius = rounded ? radius : 0
      card.clipsToBounds = rounded || card === snapshot
    }
    return radius
  }

  private func renderShadow(
    _ frames: Frames,
    radius: CGFloat
  ) {
    guard let host, let webview else { return }
    let offset = CGFloat(frames.gap == Self.filesGap ? frames.ideOffset : frames.chatOffset)
    shadow.bounds = CGRect(origin: .zero, size: host.bounds.size)
    shadow.center = CGPoint(x: host.bounds.midX + offset, y: host.bounds.midY)
    shadow.layer.cornerRadius = radius
    let beneath =
      frames.gap == 1
      ? webview.backgroundColor : (machine.position < 1 ? list : files).backgroundColor
    shadow.backgroundColor = beneath ?? .clear
    shadow.layer.shadowPath =
      UIBezierPath(roundedRect: shadow.bounds, cornerRadius: radius).cgPath
    shadow.layer.shadowOpacity = Float(frames.shadow)
    shadow.isHidden = frames.gap == nil
    dim.alpha = CGFloat(frames.dim)
    let overIde = frames.gap == 1
    dim.layer.zPosition = overIde ? Depth.scrimOverIde : Depth.scrim
    shadow.layer.zPosition = overIde ? Depth.shadowOverIde : Depth.shadow
  }

  private func park(
    _ layer: UIView,
    by offset: CGFloat
  ) {
    layer.transform = CGAffineTransform(translationX: offset, y: 0)
  }

  private func place(
    _ views: [UIView],
    as placement: HeaderPlacement
  ) {
    for view in views {
      view.transform = transform(placement.offset)
      view.alpha = CGFloat(placement.alpha)
      view.isUserInteractionEnabled = placement.alpha >= 1
      view.layer.zPosition = placement.depth
    }
  }

  private func placeCatcher(
    width: CGFloat,
    height: CGFloat
  ) {
    let resting = machine.phase == .rest ? Int(machine.position.rounded()) : -1
    let closesList = Screen.chatList.rawValue
    let closesFiles = Screen.files.rawValue
    catcher.isHidden = resting != closesList && resting != closesFiles
    switch resting {
    case closesList:
      catcher.frame = CGRect(x: width - Self.strip, y: 0, width: Self.strip, height: height)
      catcher.accessibilityLabel = "Close chat list"
    case closesFiles:
      catcher.frame = CGRect(x: 0, y: 0, width: Self.strip, height: height)
      catcher.accessibilityLabel = "Close files"
    default:
      break
    }
    // PLATFORM: ios: The status bar's glyphs are drawn for the drawer, so the card's strip
    // PLATFORM: ios: under them takes the drawer's colour; the card's own white would swallow
    // PLATFORM: ios: the clock where it straddles the edge.
    catcher.band.backgroundColor = (resting == closesFiles ? files : list).backgroundColor
    catcher.band.frame = CGRect(
      x: 0,
      y: 0,
      width: Self.strip,
      height: host?.safeAreaInsets.top ?? 0
    )
  }
}
