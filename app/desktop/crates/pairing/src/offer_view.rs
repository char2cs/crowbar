//! The Add a device dialog: a one-time code with the address and fingerprint a device confirms.

use std::rc::Rc;
use std::sync::Arc;

mod offer_body;
mod offer_source;

use gpui::{Context, EventEmitter, IntoElement, ParentElement, Render, Styled, Window, div};
use store::{DeviceOffer, OfferState};
use ui::{Button, ButtonSize, ButtonVariant, Dialog, DialogSize, Surface, Theme};

use crate::event::Event;
use crate::look::GAP;
use crate::offer_view::offer_source::OfferSource;

const TITLE: &str = "Add a device";

/// The Add a device dialog; it emits [`Event::Close`] when it is dismissed or done.
pub struct OfferView {
    source: Rc<dyn OfferSource>,
}

impl std::fmt::Debug for OfferView {
    fn fmt(
        &self,
        formatter: &mut std::fmt::Formatter<'_>,
    ) -> std::fmt::Result {
        formatter.debug_struct("OfferView").finish_non_exhaustive()
    }
}

impl EventEmitter<Event> for OfferView {}

impl OfferView {
    /// A dialog showing the code `offer` mints.
    pub fn new(offer: Arc<DeviceOffer>) -> Self {
        Self::over(Rc::new(offer))
    }

    fn over(source: Rc<dyn OfferSource>) -> Self {
        Self { source }
    }

    fn new_code(&mut self) {
        if self.source.state() != OfferState::Minting {
            self.source.renew();
        }
    }

    fn close(
        _: &mut Self,
        cx: &mut Context<'_, Self>,
    ) {
        cx.emit(Event::Close);
    }
}

impl Render for OfferView {
    fn render(
        &mut self,
        _window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) -> impl IntoElement {
        let state = self.source.state();
        let palette = *Theme::of(cx).palette(Surface::Window);
        let mono = Theme::of(cx).mono_family();
        let mut renew = Button::new("device-offer-new-code", "New code")
            .variant(ButtonVariant::Ghost)
            .size(ButtonSize::Sm)
            .on_click(cx.listener(|this, _, _, _| this.new_code()));
        if state == OfferState::Minting {
            renew = renew.disabled();
        }
        let done = Button::new("device-offer-done", "Done")
            .size(ButtonSize::Sm)
            .on_click(cx.listener(|this, _, _, cx| Self::close(this, cx)));
        let me = cx.entity();
        Dialog::new("device-offer")
            .title(TITLE)
            .size(DialogSize::Sm)
            .body(div().child(offer_body::body(&state, (&palette, mono))))
            .footer(div().flex().gap(GAP).child(renew).child(done))
            .on_close(move |_, cx| me.update(cx, Self::close))
    }
}

#[cfg(test)]
mod tests {
    use std::cell::RefCell;

    use gpui::{AppContext as _, Entity, TestAppContext};

    use super::*;

    struct Fake {
        state: RefCell<OfferState>,
        renewed: RefCell<usize>,
    }

    impl OfferSource for Fake {
        fn state(&self) -> OfferState {
            self.state.borrow().clone()
        }

        fn renew(&self) {
            *self.renewed.borrow_mut() += 1;
        }
    }

    fn open(
        cx: &mut TestAppContext,
        state: OfferState,
    ) -> (Rc<Fake>, Entity<OfferView>) {
        let fake = Rc::new(Fake {
            state: RefCell::new(state),
            renewed: RefCell::new(0),
        });
        let source: Rc<dyn OfferSource> = Rc::clone(&fake) as _;
        let view = cx.new(|_| OfferView::over(source));
        (fake, view)
    }

    #[gpui::test]
    fn a_new_code_is_asked_for_only_once_the_last_one_settled(cx: &mut TestAppContext) {
        let (fake, view) = open(cx, OfferState::Minting);
        view.update(cx, |this, _| this.new_code());
        *fake.state.borrow_mut() = OfferState::Failed;
        view.update(cx, |this, _| this.new_code());
        assert_eq!(*fake.renewed.borrow(), 1);
    }

    #[gpui::test]
    fn done_closes_the_dialog(cx: &mut TestAppContext) {
        let (_, view) = open(cx, OfferState::Failed);
        let closed = Rc::new(RefCell::new(Vec::new()));
        let sink = Rc::clone(&closed);
        let record = move |_, event: &Event, _: &mut gpui::App| sink.borrow_mut().push(*event);
        cx.update(|cx| cx.subscribe(&view, record).detach());
        view.update(cx, OfferView::close);
        assert_eq!(*closed.borrow(), [Event::Close]);
    }
}
