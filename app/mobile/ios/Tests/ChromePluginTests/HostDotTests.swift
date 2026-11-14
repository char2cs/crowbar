import SwiftUI
import XCTest

@testable import tauri_plugin_crowbar_chrome

final class HostDotTests: XCTestCase {
  private let cases: [(state: String, tone: HostDot.Tone, words: String)] = [
    ("up", .online, "Online"),
    ("no-live-stream", .online, "Online, no live stream"),
    ("down", .offline, "Offline"),
    ("not-available", .offline, "Host not available"),
    ("needs-repair", .needsAttention, "Needs pairing again"),
    ("probing", .unsettled, "Checking"),
    ("something-new", .unsettled, "Status unknown"),
  ]

  func testEveryHostStateHasATonePhrase() {
    for item in cases {
      XCTAssertEqual(HostDot.tone(item.state), item.tone, item.state)
      XCTAssertEqual(HostDot.words(item.state), item.words, item.state)
    }
  }

  func testEachToneHasItsOwnColour() {
    let tones: [HostDot.Tone] = [.online, .offline, .needsAttention, .unsettled]
    XCTAssertEqual(Set(tones.map(HostDot.color)).count, tones.count)
  }
}
