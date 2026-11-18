use std::rc::Rc;

use gpui::prelude::FluentBuilder;
use gpui::{
    AnyElement, App, CursorStyle, ElementId, FontWeight, Hsla, InteractiveElement, IntoElement,
    MouseButton, ParentElement, RenderOnce, StatefulInteractiveElement, Styled, Window, div, px,
};
use ui::{Icon, IconName, IconWeight, Palette, Spinner, Surface, Theme, radius, shadow, text_size};

use crate::member_kind::MemberKind;
use crate::member_view::MemberView;
use crate::shell_look::ShellLook;
use crate::strip_metrics::TAB_HEIGHT;

const GLYPH: f32 = 16.0;
const SPINNER: f32 = 14.0;
const CLOSE: f32 = 24.0;
const CLOSE_GLYPH: f32 = 12.0;
const CLOSE_INSET: f32 = 6.0;
const ROW_PADDING: f32 = 4.0;
const ROW_GAP: f32 = 6.0;
const DIVIDER_HEIGHT: f32 = 16.0;
const DIVIDER_ALPHA: f32 = 0.15;
const IDLE_SHADOW: f32 = 0.10;
const LIFTED_SHADOW: f32 = 0.15;
const HOVER_FILL: f32 = 0.08;

pub(crate) type CloseAction = Rc<dyn Fn(&mut Window, &mut App)>;

#[derive(IntoElement)]
pub(crate) struct TabShell {
    pub(crate) id: ElementId,
    pub(crate) members: Vec<MemberView>,
    pub(crate) look: ShellLook,
    pub(crate) width: f32,
    pub(crate) on_close: Option<CloseAction>,
}

impl RenderOnce for TabShell {
    fn render(
        self,
        _window: &mut Window,
        cx: &mut App,
    ) -> impl IntoElement {
        let palette = *Theme::of(cx).palette(Surface::Chrome);
        let look = self.look;
        let count = self.members.len();
        let members: Vec<_> = self
            .members
            .into_iter()
            .enumerate()
            .map(|(index, member)| member_row(&self.id, (index, count), member, (look, &palette)))
            .collect();
        shell(self.id.clone(), self.width, look, &palette)
            .font_family(Theme::of(cx).ui_family())
            .children(members)
            .when_some(self.on_close, |shell, close| {
                shell.child(close_button(&self.id, look, &palette, close))
            })
    }
}

fn shell(
    id: ElementId,
    width: f32,
    look: ShellLook,
    palette: &Palette,
) -> gpui::Stateful<gpui::Div> {
    let base = div()
        .id(id)
        .relative()
        .flex()
        .items_center()
        .flex_none()
        .w(px(width))
        .h(px(TAB_HEIGHT))
        .rounded(radius::FULL)
        .border_1()
        .border_color(gpui::transparent_black())
        .cursor(CursorStyle::PointingHand);
    match look {
        ShellLook::Idle => base.hover(|style| {
            style
                .bg(palette.sidebar_element_hover())
                .shadow(shadow::xs(IDLE_SHADOW))
        }),
        ShellLook::Active => base
            .bg(palette.control_surface())
            .border_color(palette.control_surface())
            .shadow(shadow::xs(IDLE_SHADOW)),
        ShellLook::Lifted => base
            .bg(palette.control_surface())
            .border_color(palette.border())
            .shadow(shadow::md(LIFTED_SHADOW)),
        ShellLook::MergeTarget => base
            .bg(palette.sidebar_drop_nest())
            .border_color(palette.sidebar_drop_nest_edge()),
    }
}

fn ink(
    look: ShellLook,
    palette: &Palette,
) -> (Hsla, Hsla) {
    match look {
        ShellLook::Active | ShellLook::Lifted => (
            palette.control_foreground(),
            palette.control_muted_foreground(),
        ),
        ShellLook::Idle | ShellLook::MergeTarget => {
            (palette.foreground(), palette.muted_foreground())
        },
    }
}

fn member_ink(
    look: ShellLook,
    strong: Hsla,
    muted: Hsla,
) -> Hsla {
    if look == ShellLook::Idle {
        return muted;
    }
    strong
}

fn spinner_glyph(spinner_id: ElementId) -> AnyElement {
    Spinner::new(spinner_id)
        .size(px(SPINNER))
        .on(Surface::Chrome)
        .into_any_element()
}

fn resting_glyph(
    kind: MemberKind,
    color: Hsla,
) -> AnyElement {
    let icon = match kind {
        MemberKind::Chat => IconName::ChatsCircle,
        MemberKind::Terminal => IconName::Terminal,
        MemberKind::NewTab => IconName::Plus,
    };
    Icon::new(icon)
        .weight(IconWeight::Regular)
        .size(px(GLYPH))
        .color(color)
        .into_any_element()
}

fn member_row(
    id: &ElementId,
    place: (usize, usize),
    member: MemberView,
    style: (ShellLook, &Palette),
) -> impl IntoElement {
    let ((index, count), (look, palette)) = (place, style);
    let (strong, muted) = ink(look, palette);
    let text = member_ink(look, strong, muted);
    let last = index + 1 == count;
    let spinner = member
        .working
        .then(|| ElementId::from((id.clone(), format!("spin{index}"))));
    let resting = (count == 1).then(|| resting_glyph(member.kind, text));
    let glyph = spinner.map(spinner_glyph).or(resting);
    div()
        .relative()
        .flex()
        .flex_1()
        .min_w_0()
        .items_center()
        .gap(px(ROW_GAP))
        .h(px(TAB_HEIGHT))
        .px(px(ROW_PADDING))
        .when(last, |row| row.pr(px(CLOSE + CLOSE_INSET)))
        .when(!last && look == ShellLook::Idle, |row| {
            row.child(divider(muted))
        })
        .children(glyph)
        .child(label(member, text))
}

fn label(
    member: MemberView,
    color: Hsla,
) -> impl IntoElement {
    div()
        .flex_1()
        .min_w_0()
        .overflow_hidden()
        .whitespace_nowrap()
        .text_ellipsis()
        .debug_selector(|| "tab-label".into())
        .text_size(text_size::SM - px(1.0))
        .font_weight(FontWeight::MEDIUM)
        .text_color(color)
        .when(member.provisional, Styled::italic)
        .child(member.label)
}

fn divider(color: Hsla) -> impl IntoElement {
    div()
        .absolute()
        .right_0()
        .top(px((TAB_HEIGHT - DIVIDER_HEIGHT) / 2.0 - 1.0))
        .w(px(1.0))
        .h(px(DIVIDER_HEIGHT))
        .bg(color.opacity(DIVIDER_ALPHA))
}

fn close_button(
    id: &ElementId,
    look: ShellLook,
    palette: &Palette,
    close: CloseAction,
) -> impl IntoElement {
    let (strong, muted) = ink(look, palette);
    let hover_fill = match look {
        ShellLook::Active | ShellLook::Lifted => strong.opacity(HOVER_FILL),
        ShellLook::Idle | ShellLook::MergeTarget => palette.sidebar_element_hover(),
    };
    div()
        .id(ElementId::from((id.clone(), "close")))
        .absolute()
        .right(px(CLOSE_INSET))
        .top(px((TAB_HEIGHT - CLOSE) / 2.0 - 1.0))
        .size(px(CLOSE))
        .flex()
        .items_center()
        .justify_center()
        .rounded(radius::FULL)
        .cursor(CursorStyle::PointingHand)
        .hover(move |style| style.bg(hover_fill))
        .on_mouse_down(MouseButton::Left, |_, _, cx| cx.stop_propagation())
        .on_click(move |_, window, cx| close(window, cx))
        .child(Icon::new(IconName::X).size(px(CLOSE_GLYPH)).color(muted))
}

#[cfg(test)]
mod tests {
    use gpui::{Context, Render, TestAppContext};
    use protocol::PaneId;
    use ui::Motion;

    use super::*;

    struct Pill {
        labels: &'static [&'static str],
    }

    impl Render for Pill {
        fn render(
            &mut self,
            _window: &mut Window,
            _cx: &mut Context<'_, Self>,
        ) -> impl IntoElement {
            let members = self
                .labels
                .iter()
                .enumerate()
                .map(|(index, label)| MemberView {
                    pane: PaneId::new(format!("pane{index}")).expect("id"),
                    label: (*label).into(),
                    provisional: false,
                    working: false,
                    kind: MemberKind::Chat,
                });
            div().flex().child(TabShell {
                id: ElementId::from("pill"),
                members: members.collect(),
                look: ShellLook::Idle,
                width: 236.0,
                on_close: None,
            })
        }
    }

    fn label_width(
        cx: &mut TestAppContext,
        labels: &'static [&'static str],
    ) -> f32 {
        cx.update(|cx| {
            ui::init(cx).expect("ui init");
            cx.set_global(Theme::default());
            cx.set_global(Motion::default());
        });
        let (_, cx) = cx.add_window_view(|_, _| Pill { labels });
        cx.run_until_parked();
        let bounds = cx.debug_bounds("tab-label");
        bounds.map_or(0.0, |bounds| f32::from(bounds.size.width))
    }

    #[gpui::test]
    fn the_members_of_a_set_leave_their_labels_room_instead_of_a_resting_glyph(
        cx: &mut TestAppContext
    ) {
        const SET: &[&str] = &["New chat", "Terminal", "Review"];
        const LONE: &[&str] = &["New chat"];
        let set = label_width(cx, SET);
        assert!(set >= 60.0, "a label of three: {set}");
        assert!(label_width(cx, LONE) >= 150.0, "a lone tab keeps its room");
    }

    #[test]
    fn the_pill_is_thirty_two_pixels_high_and_the_close_button_twenty_four() {
        assert_eq!(TAB_HEIGHT, 32.0, "pill height");
        assert_eq!(CLOSE, 24.0, "close button");
    }
}
