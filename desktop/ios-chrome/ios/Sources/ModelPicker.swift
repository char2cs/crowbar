import SwiftUI
import UIKit

struct PickerModel: Decodable, Identifiable, Equatable {
  let id: String
  let name: String
  let detail: String?
}

struct PickerProvider: Decodable, Identifiable, Equatable {
  let id: String
  let name: String
  /// A PNG data URL; `fallbackIcon` (a Phosphor name) stands in without one.
  let icon: String?
  let fallbackIcon: String
  let models: [PickerModel]
}

struct PickerSelection: Decodable, Equatable {
  let providerId: String
  let modelId: String
}

/// The composer's model and effort picker, as `set_model_picker` hands it over.
struct ModelPickerState: Decodable, Equatable {
  let open: Bool
  let providers: [PickerProvider]
  let selected: PickerSelection?
  let efforts: [String]
  let effort: String?
}

/// What the web last handed the picker, with each provider's icon decoded once.
final class ModelPickerModel: ObservableObject {
  @Published var state = ModelPickerState(
    open: false, providers: [], selected: nil, efforts: [], effort: nil)
  private(set) var icons: [String: UIImage] = [:]
  var send: (ChromeIntent) -> Void = { _ in }

  func apply(_ next: ModelPickerState) {
    for provider in next.providers where icons[provider.id] == nil {
      icons[provider.id] = provider.icon.flatMap(Self.image)
    }
    state = next
  }

  private static func image(dataURL: String) -> UIImage? {
    guard let comma = dataURL.firstIndex(of: ","),
      let data = Data(base64Encoded: String(dataURL[dataURL.index(after: comma)...]))
    else { return nil }
    return UIImage(data: data, scale: 3)
  }
}

@available(iOS 26.0, *)
struct ModelPickerSheet: View {
  @ObservedObject var model: ModelPickerModel
  /// Its palette paints the sheet as one of the web's pages.
  @ObservedObject var chrome: ChromeModel
  @State private var query = ""

  private var palette: ChromePalette? { chrome.palette }
  private var text: Color { palette?.text ?? .primary }
  private var secondaryText: Color { palette?.secondaryText ?? .secondary }

  var body: some View {
    NavigationStack {
      List {
        if !model.state.efforts.isEmpty { effort }
        ForEach(matches) { provider in
          Section {
            ForEach(provider.models) { item in row(item, of: provider) }
          } header: {
            Label { Text(provider.name) } icon: { icon(provider) }
              .foregroundStyle(secondaryText)
          }
          .listRowBackground(palette?.raised)
        }
      }
      .listStyle(.insetGrouped)
      // Opens on the Effort label, under the bar's inset, even as the catalogue lands later.
      .defaultScrollAnchor(.top)
      .scrollContentBackground(palette == nil ? .automatic : .hidden)
      .background(palette?.surface.ignoresSafeArea())
      .searchable(text: $query, placement: .navigationBarDrawer(displayMode: .always))
      .navigationTitle("Model")
      .navigationBarTitleDisplayMode(.inline)
      // Rows scroll under the bar and fade through the system's edge effect, not a hard cut.
      .toolbarBackgroundVisibility(.hidden, for: .navigationBar)
      .toolbar {
        ToolbarItem(placement: .topBarLeading) {
          Button { model.send(.of("closeModelPicker")) } label: { PhosphorIcon(.x, emphasis: 0.5).foregroundStyle(text) }
            .accessibilityLabel("Close")
        }
      }
    }
  }

  private var matches: [PickerProvider] {
    let needle = query.trimmingCharacters(in: .whitespaces)
    guard !needle.isEmpty else { return model.state.providers }
    return model.state.providers.compactMap { provider in
      let models = provider.models.filter {
        $0.name.localizedCaseInsensitiveContains(needle)
          || provider.name.localizedCaseInsensitiveContains(needle)
      }
      return models.isEmpty
        ? nil
        : PickerProvider(
          id: provider.id, name: provider.name, icon: provider.icon, fallbackIcon: provider.fallbackIcon,
          models: models)
    }
  }

  private var effort: some View {
    Section {
      EffortControl(
        levels: model.state.efforts, handed: model.state.effort, text: text,
        pill: text.opacity(0.14)
      ) { level in
        model.send(.of("setEffort", ["effort": level]))
      }
      .listRowInsets(EdgeInsets(top: 4, leading: 12, bottom: 4, trailing: 12))
    } header: {
      Text("Effort").foregroundStyle(secondaryText)
    }
    .listRowBackground(palette?.raised)
  }

  private func row(_ item: PickerModel, of provider: PickerProvider) -> some View {
    let selected = model.state.selected == PickerSelection(providerId: provider.id, modelId: item.id)
    return Button {
      Haptics.select()
      model.send(.of("selectModel", ["providerId": provider.id, "modelId": item.id]))
    } label: {
      HStack {
        VStack(alignment: .leading, spacing: 2) {
          Text(item.name).foregroundStyle(text)
          if let detail = item.detail {
            Text(detail).font(.footnote).foregroundStyle(secondaryText)
          }
        }
        Spacer()
        if selected { PhosphorIcon(.check, size: 18) }
      }
      .contentShape(.rect)
    }
    .tint(text)
    .accessibilityAddTraits(selected ? .isSelected : [])
  }

  @ViewBuilder private func icon(_ provider: PickerProvider) -> some View {
    if let image = model.icons[provider.id] {
      // Provider marks are monochrome: drawn in the theme's text colour, not the asset's own.
      Image(uiImage: image).renderingMode(.template).resizable().scaledToFit()
        .frame(width: 18, height: 18).foregroundStyle(text)
    } else {
      CrowbarIcon(named: provider.fallbackIcon).map { PhosphorIcon($0, size: 18).foregroundStyle(text) }
    }
  }
}
