import Foundation

struct ChromeHeaders: Decodable, Equatable {
  let chat: [ChromeButton]
  let ide: [ChromeButton]
}
