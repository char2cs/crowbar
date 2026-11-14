import SwiftUI
import UIKit

@available(iOS 26.0, *)
struct ModelPickerSheet: View {
  @ObservedObject var model: ModelPickerModel
  @ObservedObject var chrome: ChromeModel
  @State private var query = ""

  private var palette: ChromePalette? { chrome.palette }
  private var text: Color { palette?.text ?? .primary }
  private var secondaryText: Color { palette?.secondaryText ?? .secondary }

  var body: some View {
    NavigationStack {
      List {
        if !model.state.efforts.isEmpty { effort }
        permissionSection
        ForEach(model.state.filtered(by: query)) { provider in
          Section {
            ForEach(provider.models) { item in
              ModelRow(
                item: item,
                providerId: provider.id,
                isSelected: model.state.selected
                  == PickerSelection(providerId: provider.id, modelId: item.id),
                text: text,
                send: model.send
              )
            }
          } header: {
            Label {
              Text(provider.name)
            } icon: {
              ProviderIcon(
                image: model.icons[provider.id],
                fallbackIcon: provider.fallbackIcon,
                text: text
              )
            }
            .foregroundStyle(secondaryText)
          }
          .listRowBackground(palette?.raised)
        }
      }
      .listStyle(.insetGrouped)
      .defaultScrollAnchor(.top)
      .scrollContentBackground(palette == nil ? .automatic : .hidden)
      .background(palette?.surface.ignoresSafeArea())
      .searchable(text: $query, placement: .navigationBarDrawer(displayMode: .always))
      .navigationTitle("Model")
      .navigationBarTitleDisplayMode(.inline)
      // PLATFORM: Rows scroll under the bar and fade through the system's edge effect, not a hard cut.
      .toolbarBackgroundVisibility(.hidden, for: .navigationBar)
      .toolbar {
        ToolbarItem(placement: .topBarLeading) {
          Button {
            model.send(.of("closeModelPicker"))
          } label: {
            PhosphorIcon(.x, emphasis: 0.5).foregroundStyle(text)
          }
          .accessibilityLabel("Close")
        }
      }
    }
  }

  private var effort: some View {
    Section {
      EffortControl(
        levels: model.state.efforts,
        handed: model.state.effort,
        text: text,
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

  @ViewBuilder private var permissionSection: some View {
    if let permission = model.state.permission, !permission.levels.isEmpty {
      Section {
        ForEach(permission.levels) { item in
          Button {
            Haptics.select()
            model.send(.of("setPermissionLevel", ["level": item.value]))
          } label: {
            HStack(alignment: .top) {
              VStack(alignment: .leading, spacing: 2) {
                Text(item.label).foregroundStyle(item.value == "full-auto" ? Color.orange : text)
                Text(item.description).font(.footnote).foregroundStyle(secondaryText)
              }
              Spacer()
              if item.value == permission.level { PhosphorIcon(.check, size: 18) }
            }
            .contentShape(.rect)
          }
          .tint(text)
          .accessibilityAddTraits(item.value == permission.level ? .isSelected : [])
        }
      } header: {
        Text("Permission").foregroundStyle(secondaryText)
      }
      .listRowBackground(palette?.raised)
    }
  }

  private struct ModelRow: View {
    let item: PickerModel
    let providerId: String
    let isSelected: Bool
    let text: Color
    let send: (ChromeIntent) -> Void

    var body: some View {
      Button {
        Haptics.select()
        send(.of("selectModel", ["providerId": providerId, "modelId": item.id]))
      } label: {
        HStack {
          Text(item.name).foregroundStyle(text)
          Spacer()
          if isSelected { PhosphorIcon(.check, size: 18) }
        }
        .contentShape(.rect)
      }
      .tint(text)
      .accessibilityAddTraits(isSelected ? .isSelected : [])
    }
  }

  private struct ProviderIcon: View {
    let image: UIImage?
    let fallbackIcon: String
    let text: Color

    @ViewBuilder var body: some View {
      if let image {
        Image(uiImage: image).renderingMode(.template).resizable().scaledToFit()
          .frame(width: 18, height: 18).foregroundStyle(text)
      } else {
        CrowbarIcon(named: fallbackIcon).map { PhosphorIcon($0, size: 18).foregroundStyle(text) }
      }
    }
  }
}
