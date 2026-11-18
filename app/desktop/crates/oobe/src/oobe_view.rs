use std::rc::Rc;
use std::sync::Arc;

use gpui::{Context, EventEmitter, IntoElement, ParentElement, Render, Styled, Window, div};
use store::SystemCheck;
use ui::Theme;

use crate::backdrop::backdrop;
use crate::check_source::CheckSource;
use crate::oobe_event::OobeEvent;
use crate::oobe_screen::OobeScreen;

mod checks;
mod parts;
mod screens;

use self::parts::stage;

/// The first-run screens over the brand gradient.
pub struct OobeView {
    screen: OobeScreen,
    source: Option<Rc<dyn CheckSource>>,
}

impl std::fmt::Debug for OobeView {
    fn fmt(
        &self,
        formatter: &mut std::fmt::Formatter<'_>,
    ) -> std::fmt::Result {
        formatter
            .debug_struct("OobeView")
            .field("screen", &self.screen)
            .finish_non_exhaustive()
    }
}

impl EventEmitter<OobeEvent> for OobeView {}

impl OobeView {
    /// The first run of a computer with a daemon of its own, starting on the brand.
    #[must_use]
    pub fn first_run(check: Arc<SystemCheck>) -> Self {
        Self::over(OobeScreen::Welcome, Some(Rc::new(check)))
    }

    /// The way in for an install with no daemon of its own: connect to a host.
    #[must_use]
    pub fn add_host() -> Self {
        Self::over(OobeScreen::AddHost, None)
    }

    fn over(
        screen: OobeScreen,
        source: Option<Rc<dyn CheckSource>>,
    ) -> Self {
        Self { screen, source }
    }

    /// The screen showing now.
    #[must_use]
    pub fn screen(&self) -> OobeScreen {
        self.screen
    }

    /// Tells the view the system check answered, so it redraws.
    pub fn changed(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        cx.notify();
    }

    fn advance(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        if self.screen == OobeScreen::SystemCheck && !self.git_ready() {
            return;
        }
        self.screen = self.screen.next();
        cx.notify();
    }

    fn request_host(cx: &mut Context<'_, Self>) {
        cx.emit(OobeEvent::AddHost);
    }

    fn request_folder(cx: &mut Context<'_, Self>) {
        cx.emit(OobeEvent::ChooseFolder);
    }

    fn recheck(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        if let Some(source) = &self.source {
            source.rerun();
        }
        cx.notify();
    }
}

impl Render for OobeView {
    fn render(
        &mut self,
        _window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) -> impl IntoElement {
        let content = match self.screen {
            OobeScreen::Welcome => Self::welcome(cx),
            OobeScreen::SystemCheck => self.system_check(cx),
            OobeScreen::AddSpace => Self::add_space(cx),
            OobeScreen::AddHost => Self::host(cx),
        };
        div()
            .size_full()
            .relative()
            .font_family(Theme::of(cx).ui_family())
            .child(backdrop(self.screen))
            .child(stage(content))
    }
}

#[cfg(test)]
mod tests {
    use std::cell::RefCell;

    use gpui::{Entity, TestAppContext, VisualTestContext};

    use super::*;
    use crate::test_support::FakeCheck;

    pub(super) fn open<'a>(
        cx: &'a mut TestAppContext,
        screen: OobeScreen,
        check: &Rc<FakeCheck>,
    ) -> (Entity<OobeView>, &'a mut VisualTestContext) {
        cx.update(|cx| ui::init(cx).expect("the design system installs"));
        let source: Rc<dyn CheckSource> = Rc::clone(check) as Rc<dyn CheckSource>;
        let (view, cx) = cx.add_window_view(move |_, _| OobeView::over(screen, Some(source)));
        cx.run_until_parked();
        (view, cx)
    }

    pub(super) fn screen_of(
        view: &Entity<OobeView>,
        cx: &mut VisualTestContext,
    ) -> OobeScreen {
        view.read_with(cx, |view, _| view.screen())
    }

    fn collect(
        sink: Rc<RefCell<Vec<OobeEvent>>>
    ) -> impl FnMut(Entity<OobeView>, &OobeEvent, &mut gpui::App) + 'static {
        move |_, event, _| sink.borrow_mut().push(*event)
    }

    fn events(
        view: &Entity<OobeView>,
        cx: &mut VisualTestContext,
    ) -> Rc<RefCell<Vec<OobeEvent>>> {
        let seen = Rc::new(RefCell::new(Vec::new()));
        let sink = Rc::clone(&seen);
        cx.update(|_, cx| cx.subscribe(view, collect(sink)).detach());
        seen
    }

    #[gpui::test]
    fn the_welcome_button_leads_to_the_system_check(cx: &mut TestAppContext) {
        let check = Rc::new(FakeCheck::default());
        let (view, cx) = open(cx, OobeScreen::Welcome, &check);
        view.update(cx, OobeView::advance);
        assert_eq!(screen_of(&view, cx), OobeScreen::SystemCheck);
    }

    #[gpui::test]
    fn the_last_screens_ask_the_shell_for_what_they_offer(cx: &mut TestAppContext) {
        let check = Rc::new(FakeCheck::default());
        let (view, cx) = open(cx, OobeScreen::AddSpace, &check);
        let seen = events(&view, cx);
        view.update(cx, |_, cx| OobeView::request_folder(cx));
        view.update(cx, |_, cx| OobeView::request_host(cx));
        let expected = [OobeEvent::ChooseFolder, OobeEvent::AddHost];
        assert_eq!(seen.borrow().as_slice(), expected, "the two requests");
    }
}
