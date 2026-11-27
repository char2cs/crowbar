use std::rc::Rc;

use gpui::{App, Context, IntoElement, ParentElement, Render, Styled, Window, div, px};
use ui::{Surface, Theme};

use crate::actions::Actions;
use crate::halt_panel::HaltPanel;
use crate::report::{HaltReason, Input};

const TITLEBAR_HEIGHT: f32 = 32.0;
const STAGE_GAP: f32 = 36.0;
const WORDMARK_SIZE: f32 = 44.0;
const WORDMARK_LINE_HEIGHT_RATIO: f32 = 1.1;

/// The halt gate: once the daemon halts it replaces the window's content with why, until the
/// daemon is started again. Every other daemon state is told elsewhere, by toasts and the
/// crash panel.
pub struct View {
    halt: Option<(HaltReason, String)>,
    actions: Rc<dyn Actions>,
}

impl View {
    /// A gate with nothing to show; `actions` receives the user's commands.
    pub fn new(actions: Rc<dyn Actions>) -> Self {
        Self {
            halt: None,
            actions,
        }
    }

    /// Feeds what the daemon supervisor reported: a halt shows the gate, a restart lifts it.
    pub fn apply(
        &mut self,
        input: Input,
        cx: &mut Context<'_, Self>,
    ) {
        match input {
            Input::Halted { reason, message } => {
                self.halt = Some((HaltReason::from_code(reason.as_deref()), message));
            },
            Input::Restarted => self.halt = None,
            _ => return,
        }
        cx.notify();
    }

    /// Whether the gate covers the window, hiding the layout under it.
    pub fn covers(&self) -> bool {
        self.halt.is_some()
    }
}

impl std::fmt::Debug for View {
    fn fmt(
        &self,
        formatter: &mut std::fmt::Formatter<'_>,
    ) -> std::fmt::Result {
        formatter
            .debug_struct("View")
            .field("halt", &self.halt)
            .finish_non_exhaustive()
    }
}

fn wordmark(cx: &App) -> impl IntoElement + use<> {
    let palette = Theme::of(cx).palette(Surface::Window);
    div()
        .font_family(Theme::of(cx).heading_family())
        .text_size(px(WORDMARK_SIZE))
        .line_height(px(WORDMARK_SIZE * WORDMARK_LINE_HEIGHT_RATIO))
        .text_color(palette.foreground())
        .child("Crowbar")
}

impl Render for View {
    fn render(
        &mut self,
        _window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) -> impl IntoElement {
        let Some((reason, message)) = &self.halt else {
            return div();
        };
        let palette = Theme::of(cx).palette(Surface::Window);
        let stage = div()
            .flex()
            .flex_col()
            .items_center()
            .gap(px(STAGE_GAP))
            .child(wordmark(cx))
            .child(HaltPanel::new(reason, message.clone(), &self.actions));
        div()
            .relative()
            .size_full()
            .flex()
            .items_center()
            .justify_center()
            .bg(palette.background())
            .text_color(palette.foreground())
            .font_family(Theme::of(cx).ui_family())
            .child(stage)
            .child(ui::window_drag_strip(TITLEBAR_HEIGHT))
    }
}

#[cfg(test)]
mod tests {
    use std::rc::Rc;

    use gpui::{Entity, TestAppContext, VisualTestContext};

    use super::*;
    use crate::command::Command;
    use crate::report::{MigrationStep, Outage};
    use crate::test_support::RecordingActions;

    const SWEEP_STEP: f32 = 12.0;

    struct Harness {
        view: Entity<View>,
        actions: Rc<RecordingActions>,
    }

    fn open(cx: &mut TestAppContext) -> (Harness, &mut VisualTestContext) {
        cx.update(|cx| ui::init(cx).expect("the design system installs"));
        let actions = Rc::new(RecordingActions::default());
        let view_actions = Rc::clone(&actions);
        let (view, cx) = cx.add_window_view(move |_, _| View::new(view_actions));
        (Harness { view, actions }, cx)
    }

    fn feed(
        harness: &Harness,
        cx: &mut VisualTestContext,
        input: Input,
    ) {
        harness.view.update(cx, |view, cx| view.apply(input, cx));
        cx.run_until_parked();
    }

    fn halted() -> Input {
        Input::Halted {
            reason: Some("layout_newer".to_owned()),
            message: "layout 9 is newer than 7".to_owned(),
        }
    }

    #[gpui::test]
    fn only_a_halt_covers_the_window_until_the_daemon_is_restarted(cx: &mut TestAppContext) {
        let (harness, cx) = open(cx);
        let step = MigrationStep::new("Rebuild index", 2, 5);
        let recovering = Outage::Recovering {
            attempt: 1,
            reason: "exit 1".to_owned(),
        };
        let gave_up = Outage::GaveUp {
            report: "log".to_owned(),
        };
        let cases = [
            (Input::Starting, false),
            (Input::Migrating { step: Some(step) }, false),
            (Input::Unavailable(recovering), false),
            (Input::Unavailable(Outage::Unresponsive), false),
            (Input::Unavailable(Outage::Stopped), false),
            (Input::Unavailable(gave_up), false),
            (Input::Ready, false),
            (halted(), true),
            (Input::Ready, true),
            (Input::Restarted, false),
        ];
        for (input, covers) in cases {
            feed(&harness, cx, input.clone());
            let shown = harness.view.read_with(cx, |view, _| view.covers());
            assert_eq!(shown, covers, "{input:?}");
        }
    }

    #[gpui::test]
    fn the_halt_screen_keeps_a_strip_along_its_top_to_move_the_window_by(cx: &mut TestAppContext) {
        let (harness, cx) = open(cx);
        feed(&harness, cx, halted());
        let size = cx.update(|window, _| window.viewport_size());
        let strip = cx.debug_bounds("window-grip").expect("a grip");
        assert_eq!(strip.origin, gpui::point(px(0.0), px(0.0)));
        assert_eq!(strip.size.width, size.width);
    }

    #[gpui::test]
    fn the_halt_screen_can_quit(cx: &mut TestAppContext) {
        let (harness, cx) = open(cx);
        feed(&harness, cx, halted());
        let size = cx.update(|window, _| window.viewport_size());
        let along = |to: f32| {
            std::iter::successors(Some(0.0), |at| Some(at + SWEEP_STEP))
                .take_while(move |at| *at < to)
        };
        let under_the_strip = |y: &f32| *y > TITLEBAR_HEIGHT;
        let (width, height) = (f32::from(size.width), f32::from(size.height));
        let rows = along(height).filter(under_the_strip);
        for (x, y) in rows.flat_map(|y| along(width).map(move |x| (x, y))) {
            cx.simulate_click(gpui::point(px(x), px(y)), gpui::Modifiers::none());
        }
        let performed = harness.actions.performed();
        assert!(performed.contains(&Command::Quit), "{performed:?}");
    }
}
