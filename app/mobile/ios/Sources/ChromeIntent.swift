import Tauri

struct ChromeIntent {
  let payload: JSObject

  static func of(_ type: String, _ fields: JSObject = [:]) -> ChromeIntent {
    var payload = fields
    payload["type"] = type
    return ChromeIntent(payload: payload)
  }

  static let start = ChromeIntent.of("start")
  static let dismiss = ChromeIntent.of("dismiss")
  static let back = ChromeIntent.of("back")
  static let confirmFingerprint = ChromeIntent.of("confirmFingerprint")
  static func submitAddress(name: String, address: String) -> ChromeIntent {
    of("submitAddress", ["name": name, "address": address])
  }
  static func submitCode(_ code: String) -> ChromeIntent { of("submitCode", ["code": code]) }
  static func tap(_ id: String) -> ChromeIntent { of("tap", ["id": id]) }
}
