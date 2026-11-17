import SwiftUI
import UIKit

@available(iOS 26.0, *)
struct ModelPickerSheet: View {
  @ObservedObject var model: ModelPickerModel
  @ObservedObject var chrome: ChromeModel
  @State private var query = ""

  private static let closeEmphasis: CGFloat = 0.5

  private var palette: ChromePalette? { chrome.palette }
  private var text: Color { palette?.text ?? .primary }
  private var secondaryText: Color { palette?.secondaryText ?? .secondary }

  var body: some View {
    NavigationStack { list }
  }

  private var list: some View {
    List {
      effortSection
      permissionSection
      providerSections
    }
    .listStyle(.insetGrouped)
    .defaultScrollAnchor(.top)
    .scrollContentBackground(palette == nil ? .automatic : .hidden)
    .background(palette?.surface.ignoresSafeArea())
    .searchable(text: $query, placement: .navigationBarDrawer(displayMode: .always))
    .navigationTitle("Model")
    .navigationBarTitleDisplayMode(.inline)
    // PLATFORM: ios: Rows scroll under the bar and fade through the system's edge effect.
    .toolbarBackgroundVisibility(.hidden, for: .navigationBar)
    .toolbar { toolbarContent }
  }

  @ToolbarContentBuilder private var toolbarContent: some ToolbarContent {
    ToolbarItem(placement: .topBarLeading) { closeButton }
  }

  private var closeButton: some View {
    Button(
      action: { model.send(.of("closeModelPicker")) },
      label: { PhosphorIcon(.xMark, emphasis: Self.closeEmphasis).foregroundStyle(text) }
    )
    .accessibilityLabel("Close")
  }

  private var providerSections: some View {
    ForEach(model.state.filtered(by: query)) { provider in
      ProviderSection(
        provider: provider,
        selected: model.state.selected,
        icon: model.icons[provider.id],
        palette: palette,
        send: model.send
      )
    }
  }

  @ViewBuilder private var effortSection: some View {
    if !model.state.efforts.isEmpty { effort }
  }

  private var effort: some View {
    Section {
      effortControl
    } header: {
      Text("Effort").foregroundStyle(secondaryText)
    }
    .listRowBackground(palette?.raised)
  }

  private var effortControl: some View {
    EffortControl(
      levels: model.state.efforts,
      handed: model.state.effort,
      text: text,
      pill: text.opacity(Alpha.wash),
      onPick: setEffort
    )
    .listRowInsets(effortInsets)
  }

  private var setEffort: (String) -> Void {
    { level in model.send(.of("setEffort", ["effort": level])) }
  }

  private var effortInsets: EdgeInsets {
    EdgeInsets(
      top: Spacing.tight,
      leading: Spacing.medium,
      bottom: Spacing.tight,
      trailing: Spacing.medium
    )
  }

  @ViewBuilder private var permissionSection: some View {
    if let permission = model.state.permission, !permission.levels.isEmpty {
      PermissionSection(
        permission: permission,
        palette: palette,
        text: text,
        secondaryText: secondaryText,
        send: model.send
      )
    }
  }

  private struct PermissionSection: View {
    let permission: PickerPermission
    let palette: ChromePalette?
    let text: Color
    let secondaryText: Color
    let send: (ChromeIntent) -> Void

    var body: some View {
      Section {
        rows
      } header: {
        Text("Permission").foregroundStyle(secondaryText)
      }
      .listRowBackground(palette?.raised)
    }

    private var rows: some View {
      ForEach(permission.levels) { item in
        PermissionRow(
          item: item,
          isCurrent: item.value == permission.level,
          text: text,
          secondaryText: secondaryText,
          send: send
        )
      }
    }
  }

  private struct PermissionRow: View {
    let item: PickerPermissionLevel
    let isCurrent: Bool
    let text: Color
    let secondaryText: Color
    let send: (ChromeIntent) -> Void

    private static let titleGap: CGFloat = 2

    var body: some View {
      Button(action: choose) { label }
        .tint(text)
        .accessibilityAddTraits(isCurrent ? .isSelected : [])
    }

    private var choose: () -> Void {
      {
        Haptics.select()
        send(.of("setPermissionLevel", ["level": item.value]))
      }
    }

    private var label: some View {
      HStack(alignment: .top) {
        titles
        Spacer()
        mark
      }
      .contentShape(.rect)
    }

    private var titles: some View {
      VStack(alignment: .leading, spacing: Self.titleGap) {
        Text(item.label).foregroundStyle(item.value == "full-auto" ? Color.orange : text)
        Text(item.description).font(.footnote).foregroundStyle(secondaryText)
      }
    }

    @ViewBuilder private var mark: some View {
      if isCurrent { PhosphorIcon(.check, size: IconSize.standard) }
    }
  }

  private struct ProviderSection: View {
    let provider: PickerProvider
    let selected: PickerSelection?
    let icon: UIImage?
    let palette: ChromePalette?
    let send: (ChromeIntent) -> Void

    private var text: Color { palette?.text ?? .primary }
    private var secondaryText: Color { palette?.secondaryText ?? .secondary }

    var body: some View {
      Section {
        rows
      } header: {
        header
      }
      .listRowBackground(palette?.raised)
    }

    private var rows: some View {
      ForEach(provider.models) { item in
        ModelRow(
          item: item,
          providerId: provider.id,
          isSelected: selected == PickerSelection(providerId: provider.id, modelId: item.id),
          text: text,
          send: send
        )
      }
    }

    private var header: some View {
      Label {
        Text(provider.name)
      } icon: {
        ProviderIcon(image: icon, fallbackIcon: provider.fallbackIcon, text: text)
      }
      .foregroundStyle(secondaryText)
    }
  }

  private struct ModelRow: View {
    let item: PickerModel
    let providerId: String
    let isSelected: Bool
    let text: Color
    let send: (ChromeIntent) -> Void

    var body: some View {
      Button(action: choose) { label }
        .tint(text)
        .accessibilityAddTraits(isSelected ? .isSelected : [])
    }

    private var choose: () -> Void {
      {
        Haptics.select()
        send(.of("selectModel", ["providerId": providerId, "modelId": item.id]))
      }
    }

    private var label: some View {
      HStack {
        Text(item.name).foregroundStyle(text)
        Spacer()
        mark
      }
      .contentShape(.rect)
    }

    @ViewBuilder private var mark: some View {
      if isSelected { PhosphorIcon(.check, size: IconSize.standard) }
    }
  }

  private struct TemplateImage: View {
    let image: UIImage
    let text: Color

    var body: some View {
      Image(uiImage: image).renderingMode(.template).resizable().scaledToFit()
        .frame(width: IconSize.standard, height: IconSize.standard).foregroundStyle(text)
    }
  }

  private struct ProviderIcon: View {
    let image: UIImage?
    let fallbackIcon: String
    let text: Color

    @ViewBuilder var body: some View {
      switch image {
      case .some(let image): TemplateImage(image: image, text: text)
      case .none: fallback
      }
    }

    private var fallback: some View {
      CrowbarIcon(named: fallbackIcon)
        .map { PhosphorIcon($0, size: IconSize.standard).foregroundStyle(text) }
    }
  }
}
