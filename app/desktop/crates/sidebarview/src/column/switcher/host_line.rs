//! A remote space's host under its name: its icon and dot, its name and, short of online, its
//! state in its tone; the way into Settings → Hosts.
//!
//! Spec: develop `web/src/components/layout/sidebar-project-switcher.tsx` (`ProjectHostLine`).

use gpui::{
    AnyElement, Context, ElementId, Hsla, InteractiveElement, IntoElement, ParentElement,
    SharedString, StatefulInteractiveElement, Styled, div, px,
};
use sidebar::{Command, Host, HostTone, Intent};
use ui::{Appearance, Palette, Theme, Tooltip};

use crate::column::header::host_icon;
use crate::sidebar::RowAction;
use crate::sidebar::SPACE_XS;
use crate::sidebar::Sidebar;

const TEXT: f32 = 10.0;
const LINE: f32 = 12.0;
pub(crate) const SELECTOR: &str = "space-host-line";

pub(crate) fn host_line_text(host: &Host) -> String {
    if host.state.tone() == HostTone::Online {
        return host.name.clone();
    }
    format!("{} \u{b7} {}", host.name, host.state.label())
}

fn tone_color(
    tone: HostTone,
    (palette, appearance): (&Palette, Appearance),
) -> Hsla {
    let dark = appearance == Appearance::Dark;
    match tone {
        HostTone::Online | HostTone::Unsettled => palette.muted_foreground(),
        HostTone::Offline if dark => palette.destructive(),
        HostTone::Offline => palette.destructive_foreground(),
        HostTone::NeedsAttention if dark => palette.warning(),
        HostTone::NeedsAttention => palette.warning_foreground(),
    }
}

impl Sidebar {
    pub(crate) fn host_line(
        host: &Host,
        theme: &Theme,
        cx: &mut Context<'_, Self>,
    ) -> AnyElement {
        let palette = theme.palette(ui::Surface::Chrome);
        let color = tone_color(host.state.tone(), (palette, theme.appearance()));
        let action = RowAction::Intent(Intent::Send(Command::OpenHostSettings(host.id.clone())));
        let line = div()
            .id(SELECTOR)
            .debug_selector(|| SELECTOR.into())
            .flex()
            .min_w_0()
            .items_center()
            .gap(SPACE_XS)
            .cursor_pointer()
            .text_size(px(TEXT))
            .line_height(px(LINE))
            .text_color(color)
            .child(host_icon(host, (palette, palette.chrome_bg())))
            .child(div().truncate().child(host_line_text(host)))
            .on_click(cx.listener(move |this, _, window, cx| {
                this.act(action.clone(), window, cx);
            }));
        let tip = format!("Host settings: {}, {}", host.name, host.state.label());
        let id = ElementId::from(SharedString::from(format!("tip-{SELECTOR}")));
        Tooltip::new(id, tip, line).into_any_element()
    }
}

#[cfg(test)]
mod tests {
    use sidebar::{HostId, HostKind, HostState};

    use super::*;

    fn host(state: HostState) -> Host {
        Host {
            id: HostId::from("devbox"),
            name: "devbox".into(),
            kind: HostKind::Remote,
            state,
            endpoint: None,
        }
    }

    #[test]
    fn the_line_names_the_host_and_says_its_state_short_of_online() {
        let cases = [
            (HostState::Up, "devbox"),
            (HostState::Down, "devbox \u{b7} Offline"),
            (HostState::NeedsRepair, "devbox \u{b7} Needs pairing again"),
        ];
        for (state, text) in cases {
            assert_eq!(host_line_text(&host(state)), text, "{state:?}");
        }
    }
}
