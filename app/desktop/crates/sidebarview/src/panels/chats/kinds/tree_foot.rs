use std::rc::Rc;

use gpui::{
    App, Div, ElementId, InteractiveElement, IntoElement, ParentElement, RenderOnce, SharedString,
    Stateful, StatefulInteractiveElement, Styled, Window, div, px,
};
use sidebar::{Command, Intent, ProjectId};
use ui::{Icon, IconName, IconWeight, Palette, Surface, Theme, radius};

use crate::panels::chats::parts::{MenuAnchor, report};
use crate::sidebar::{ICON_LARGE, ROW_HEIGHT, ROW_MARGIN_X, ROW_MARGIN_Y, SPACE_SM, TEXT_BODY};
use crate::sidebar::{RowAction, RowHandler};

const DIVIDER_MARGIN_X: f32 = 12.0;
const DIVIDER_MARGIN_Y: f32 = 4.0;
const DIVIDER_ALPHA: f32 = 0.15;
const ICON_ALPHA: f32 = 0.83;
const LABEL_NUDGE: f32 = 1.5;

#[derive(IntoElement)]
pub(crate) struct TreeFoot {
    pub(crate) project: ProjectId,
    pub(crate) handler: RowHandler,
}

impl RenderOnce for TreeFoot {
    fn render(
        self,
        _window: &mut Window,
        cx: &mut App,
    ) -> impl IntoElement {
        let palette = Theme::of(cx).palette(Surface::Chrome);
        let divider = div()
            .mx(px(DIVIDER_MARGIN_X))
            .my(px(DIVIDER_MARGIN_Y))
            .h(px(1.0))
            .bg(palette.foreground().opacity(DIVIDER_ALPHA));
        div()
            .child(divider)
            .child(self.new_thread(palette))
            .child(self.more(palette))
    }
}

impl TreeFoot {
    fn new_thread(
        &self,
        palette: &'static Palette,
    ) -> Stateful<Div> {
        let id = format!("foot-new-thread-{}", self.project.as_str());
        let action = RowAction::Intent(Intent::Send(Command::NewThread(self.project.clone())));
        foot_row(id, palette)
            .child(foot_icon(IconName::Plus, IconWeight::Regular, palette))
            .child(foot_label("New thread"))
            .on_click(report(&self.handler, action))
    }

    fn more(
        &self,
        palette: &'static Palette,
    ) -> Stateful<Div> {
        let id = format!("foot-more-{}", self.project.as_str());
        let (project, handler) = (self.project.clone(), Rc::clone(&self.handler));
        let anchor = MenuAnchor::default();
        foot_row(id, palette)
            .relative()
            .debug_selector(|| "foot-more".into())
            .child(anchor.probe())
            .child(foot_icon(IconName::DotsThree, IconWeight::Bold, palette))
            .child(foot_label("More…"))
            .on_click(move |_, window, cx| {
                handler(foot_menu_action(&project, anchor.below()), window, cx);
            })
    }
}

fn foot_menu_action(
    project: &ProjectId,
    at: gpui::Point<gpui::Pixels>,
) -> RowAction {
    RowAction::FootMenu {
        project: project.clone(),
        at,
    }
}

fn foot_row(
    id: String,
    palette: &'static Palette,
) -> Stateful<Div> {
    div()
        .id(ElementId::from(SharedString::from(id)))
        .flex()
        .items_center()
        .gap(SPACE_SM)
        .h(px(ROW_HEIGHT))
        .mx(px(ROW_MARGIN_X))
        .mt(px(ROW_MARGIN_Y))
        .px(SPACE_SM)
        .rounded(radius::LG)
        .border_1()
        .border_color(gpui::transparent_black())
        .text_size(TEXT_BODY)
        .font_weight(gpui::FontWeight::MEDIUM)
        .text_color(palette.muted_foreground())
        .cursor_pointer()
        .hover(move |style| style.bg(palette.sidebar_element_idle()))
}

fn foot_icon(
    name: IconName,
    weight: IconWeight,
    palette: &Palette,
) -> Icon {
    Icon::new(name)
        .weight(weight)
        .size(ICON_LARGE)
        .color(palette.muted_foreground().opacity(ICON_ALPHA))
}

fn foot_label(text: &'static str) -> Div {
    div().relative().top(px(LABEL_NUDGE)).child(text)
}

#[cfg(test)]
mod tests {
    use std::rc::Rc;

    use gpui::{Context, Render, TestAppContext, point, px};

    use super::*;
    use crate::test_support::{measure, recording_handler};

    #[gpui::test]
    fn the_foot_stacks_two_rows_under_a_divider(cx: &mut TestAppContext) {
        let (handler, _) = recording_handler();
        let bounds = measure(cx, move |_, _| TreeFoot {
            project: ProjectId::from("inv"),
            handler: Rc::clone(&handler),
        });
        let rows = 2.0 * (ROW_HEIGHT + ROW_MARGIN_Y);
        assert!(
            f32::from(bounds.size.height) > rows,
            "the divider adds height"
        );
    }

    #[gpui::test]
    fn the_foot_rows_stay_inside_their_column(cx: &mut TestAppContext) {
        let (handler, _) = recording_handler();
        cx.update(|cx| ui::init(cx).expect("ui installs"));
        let (_, cx) = cx.add_window_view(|_, _| Foot { handler });
        cx.simulate_resize(gpui::size(px(300.0), px(200.0)));
        cx.run_until_parked();
        let more = cx.debug_bounds("foot-more").unwrap_or_default();
        assert_eq!(
            (more.left(), more.right()),
            (px(ROW_MARGIN_X), px(300.0 - ROW_MARGIN_X))
        );
    }

    struct Foot {
        handler: RowHandler,
    }

    impl Render for Foot {
        fn render(
            &mut self,
            _: &mut Window,
            _: &mut Context<'_, Self>,
        ) -> impl IntoElement {
            TreeFoot {
                project: ProjectId::from("inv"),
                handler: Rc::clone(&self.handler),
            }
        }
    }

    #[gpui::test]
    fn clicking_new_thread_asks_for_a_thread_in_the_project(cx: &mut TestAppContext) {
        let (handler, seen) = recording_handler();
        cx.update(|cx| ui::init(cx).expect("ui installs"));
        let (_, cx) = cx.add_window_view(|_, _| Foot { handler });
        cx.simulate_click(
            point(px(60.0), px(DIVIDER_MARGIN_Y + ROW_HEIGHT / 2.0 + 4.0)),
            gpui::Modifiers::none(),
        );
        let expected = RowAction::Intent(Intent::Send(Command::NewThread(ProjectId::from("inv"))));
        assert_eq!(seen.borrow().first(), Some(&expected));
    }

    #[gpui::test]
    fn more_opens_its_menu_under_the_left_edge_of_the_row_wherever_it_is_clicked(
        cx: &mut TestAppContext
    ) {
        let (handler, seen) = recording_handler();
        cx.update(|cx| ui::init(cx).expect("ui installs"));
        let (_, cx) = cx.add_window_view(|_, _| Foot { handler });
        let more_y = DIVIDER_MARGIN_Y + 2.0 * (ROW_MARGIN_Y + ROW_HEIGHT) - ROW_HEIGHT / 2.0;
        cx.simulate_click(point(px(150.0), px(more_y)), gpui::Modifiers::none());
        let Some(RowAction::FootMenu { at, .. }) = seen.borrow().first().cloned() else {
            panic!("More… asked for no menu: {:?}", seen.borrow());
        };
        let more = cx.debug_bounds("foot-more").unwrap_or_default();
        let (dx, dy) = (at.x - more.left(), at.y - (more.bottom() + px(4.0)));
        assert!(
            dx.abs() <= px(1.0) && dy.abs() <= px(1.0),
            "{at:?} under {more:?}"
        );
    }
}
