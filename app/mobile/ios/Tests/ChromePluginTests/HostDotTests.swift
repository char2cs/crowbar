import SwiftUI
import XCTest

@testable import tauri_plugin_crowbar_chrome

final class HostDotTests: XCTestCase {
  private struct HostCase {
    let state: String
    let tone: HostDot.Tone
    let words: String
  }

  private let cases = [
    HostCase(state: "up", tone: .online, words: "Online"),
    HostCase(state: "no-live-stream", tone: .online, words: "Online, no live stream"),
    HostCase(state: "down", tone: .offline, words: "Offline"),
    HostCase(state: "not-available", tone: .offline, words: "Host not available"),
    HostCase(state: "needs-repair", tone: .needsAttention, words: "Needs pairing again"),
    HostCase(state: "probing", tone: .unsettled, words: "Checking"),
    HostCase(state: "something-new", tone: .unsettled, words: "Status unknown"),
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
