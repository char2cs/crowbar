import Foundation

struct ChromeButton: Decodable, Identifiable, Equatable {
  let id: String
  let icon: String
  let side: String
  let label: String
  let enabled: Bool
  let intent: String?
}
