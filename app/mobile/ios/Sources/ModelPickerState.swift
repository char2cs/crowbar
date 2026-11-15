import Foundation

struct ModelPickerState: Decodable, Equatable {
  let open: Bool
  let providers: [PickerProvider]
  let selected: PickerSelection?
  let efforts: [String]
  let effort: String?
  let permission: PickerPermission?

  func filtered(by query: String) -> [PickerProvider] {
    let needle = query.trimmingCharacters(in: .whitespaces)
    guard !needle.isEmpty else { return providers }
    return providers.compactMap { narrowed($0, to: needle) }
  }

  private func narrowed(
    _ provider: PickerProvider,
    to needle: String
  ) -> PickerProvider? {
    let models = provider.models.filter {
      $0.name.localizedCaseInsensitiveContains(needle)
        || provider.name.localizedCaseInsensitiveContains(needle)
    }
    guard !models.isEmpty else { return nil }
    return PickerProvider(
      id: provider.id,
      name: provider.name,
      icon: provider.icon,
      fallbackIcon: provider.fallbackIcon,
      models: models
    )
  }
}
