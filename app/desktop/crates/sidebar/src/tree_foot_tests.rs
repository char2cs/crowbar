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

struct Foot {
    handler: RowHandler,
}

impl Render for Foot {
    fn render(&mut self, _: &mut Window, _: &mut Context<'_, Self>) -> impl IntoElement {
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
