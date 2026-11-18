//! The hover-revealed buttons at the end of a tree row.

use gpui::{
    Div, ElementId, InteractiveElement, IntoElement, MouseButton, ParentElement, SharedString,
    Stateful, StatefulInteractiveElement, StyleRefinement, Styled, div, px,
};
use sidebar::{Expansion, Intent, Nesting, RowId};
use ui::{Icon, IconName, IconWeight, radius};

use crate::cluster_spec::ClusterSpec;
use crate::row_action::RowAction;
use crate::row_report::report;
use crate::row_verb::RowVerb;

const BUTTON_PADDING: f32 = 6.0;
const VERB_ICON: f32 = 12.0;
const MENU_ICON: f32 = 14.0;
const CLUSTER_INSET: f32 = 6.0;
const CLUSTER_GAP: f32 = 6.0;

pub(crate) fn cluster(spec: &ClusterSpec<'_>) -> Div {
    let buttons = RowVerb::for_row(spec.row)
        .into_iter()
        .map(|verb| verb_button(spec, verb));
    let fold = (spec.row.nesting == Nesting::Parent).then(|| fold_button(spec));
    let palette = spec.palette;
    div()
        .absolute()
        .right(px(CLUSTER_INSET))
        .top_0()
        .bottom_0()
        .invisible()
        .group_hover(spec.group.clone(), StyleRefinement::visible)
        .flex()
        .items_center()
        .gap(px(CLUSTER_GAP))
        .pl(px(CLUSTER_GAP))
        .bg(palette.chrome_bg().blend(palette.sidebar_element_idle()))
        .rounded(radius::LG)
        .children(buttons)
        .children(fold)
}

fn button_frame(
    id: ElementId,
    spec: &ClusterSpec<'_>,
) -> Stateful<Div> {
    let palette = spec.palette;
    let hovered = move |style: StyleRefinement| {
        style
            .bg(palette.sidebar_element_hover())
            .text_color(palette.foreground())
    };
    div()
        .id(id)
        .flex()
        .flex_none()
        .items_center()
        .justify_center()
        .p(px(BUTTON_PADDING))
        .rounded(radius::LG)
        .cursor_pointer()
        .text_color(palette.muted_foreground())
        .hover(hovered)
        .on_mouse_down(MouseButton::Left, |_, window, cx| {
            window.prevent_default();
            cx.stop_propagation();
        })
}

fn verb_button(
    spec: &ClusterSpec<'_>,
    verb: RowVerb,
) -> impl IntoElement {
    let (name, weight) = verb.icon();
    let size = match verb {
        RowVerb::RepoMenu => px(MENU_ICON),
        _ => px(VERB_ICON),
    };
    let id = ElementId::from(SharedString::from(format!("verb-{}-{verb:?}", spec.group)));
    let frame = button_frame(id, spec).child(Icon::new(name).weight(weight).size(size));
    match verb.intent(spec.row) {
        Some(intent) => frame.on_click(report(&spec.handler, RowAction::Intent(intent))),
        None => frame.on_click(repo_menu(spec)),
    }
}

fn repo_menu_action(
    row: &RowId,
    at: gpui::Point<gpui::Pixels>,
) -> RowAction {
    RowAction::RepoMenu {
        row: row.clone(),
        at,
    }
}

fn repo_menu(
    spec: &ClusterSpec<'_>
) -> impl Fn(&gpui::ClickEvent, &mut gpui::Window, &mut gpui::App) + 'static {
    let (handler, row) = (std::rc::Rc::clone(&spec.handler), spec.row.id.clone());
    move |event, window, cx| {
        cx.stop_propagation();
        handler(repo_menu_action(&row, event.position()), window, cx);
    }
}

fn fold_button(spec: &ClusterSpec<'_>) -> impl IntoElement {
    let id = ElementId::from(SharedString::from(format!("fold-{}", spec.group)));
    let turn = match spec.expansion {
        Expansion::Expanded => std::f32::consts::FRAC_PI_2,
        Expansion::Collapsed => 0.0,
    };
    let action = RowAction::Intent(Intent::ToggleFold(spec.row.id.clone()));
    button_frame(id, spec)
        .child(
            Icon::new(IconName::CaretRight)
                .weight(IconWeight::Bold)
                .size(px(VERB_ICON))
                .rotate(turn),
        )
        .on_click(report(&spec.handler, action))
}

#[cfg(test)]
mod tests {
    use std::rc::Rc;

    use gpui::{SharedString, TestAppContext};
    use ui::{Surface, Theme};

    use super::*;
    use sidebar::{Expansion, Row};

    use crate::row_action::RowHandler;
    use crate::test_support::{chat, measure, recording_handler};

    fn spec_of<'a>(
        row: &'a Row,
        palette: &'static ui::Palette,
        handler: &RowHandler,
    ) -> ClusterSpec<'a> {
        ClusterSpec {
            row,
            group: SharedString::from("row"),
            expansion: Expansion::Expanded,
            palette,
            handler: Rc::clone(handler),
        }
    }

    #[gpui::test]
    fn the_cluster_is_laid_out_without_taking_flow_space(cx: &mut TestAppContext) {
        let (handler, _) = recording_handler();
        let row = chat("Refactor");
        let bounds = measure(cx, move |_, cx| {
            let palette = Theme::of(cx).palette(Surface::Chrome);
            cluster(&spec_of(&row, palette, &handler))
        });
        assert_eq!(f32::from(bounds.size.height), 0.0);
    }
}
