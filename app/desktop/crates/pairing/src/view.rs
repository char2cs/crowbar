//! The dialog: name and address, fingerprint, then the one-time code.

use std::rc::Rc;
use std::sync::Arc;

use gpui::{
    Context, Div, Entity, EventEmitter, IntoElement, ParentElement, Render, Styled, Subscription,
    Window, div,
};
use store::{PairingPending, PairingSession, PairingStage, PairingState};
use ui::{Dialog, DialogSize, Palette, Surface, TextInput, TextInputEvent, Theme};

use crate::action::Action;
use crate::event::Event;
use crate::look::{ADDRESS_PLACEHOLDER, CODE_PLACEHOLDER, GAP, NAME_PLACEHOLDER, TITLE};
use crate::source::Source;
use crate::stage_copy;
use crate::stage_widgets;

const FIELDS: usize = 3;

/// The Add Host dialog; it emits a [`Event`] when the host is paired or it is dismissed.
pub struct View {
    source: Rc<dyn Source>,
    name: Entity<TextInput>,
    address: Entity<TextInput>,
    code: Entity<TextInput>,
    seen: PairingStage,
    _inputs: [Subscription; FIELDS],
}

impl std::fmt::Debug for View {
    fn fmt(
        &self,
        formatter: &mut std::fmt::Formatter<'_>,
    ) -> std::fmt::Result {
        formatter
            .debug_struct("View")
            .field("seen", &self.seen)
            .finish_non_exhaustive()
    }
}

impl EventEmitter<Event> for View {}

impl View {
    /// A dialog over `session`, with the name field focused.
    pub fn new(
        session: Arc<PairingSession>,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) -> Self {
        Self::over(Rc::new(session), window, cx)
    }

    fn over(
        source: Rc<dyn Source>,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) -> Self {
        let name = stage_widgets::field(NAME_PLACEHOLDER, cx);
        let address = stage_widgets::field(ADDRESS_PLACEHOLDER, cx);
        let code = stage_widgets::field(CODE_PLACEHOLDER, cx);
        name.update(cx, |field, cx| field.focus(window, cx));
        let inputs = [&name, &address, &code].map(|input| cx.subscribe(input, Self::on_input));
        let seen = source.state().stage;
        Self {
            source,
            name,
            address,
            code,
            seen,
            _inputs: inputs,
        }
    }

    /// Tells the dialog the session moved: it redraws and announces a finished pairing once.
    pub fn changed(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        let stage = self.source.state().stage;
        let arrived = stage == PairingStage::Paired && self.seen != PairingStage::Paired;
        self.seen = stage;
        if arrived {
            cx.emit(Event::Paired);
        }
        cx.notify();
    }

    fn on_input(
        &mut self,
        _: Entity<TextInput>,
        event: &TextInputEvent,
        cx: &mut Context<'_, Self>,
    ) {
        match event {
            TextInputEvent::Changed(_) => self.source.act(Action::Edited),
            TextInputEvent::Submitted => self.advance(cx),
            TextInputEvent::Cancelled => self.dismiss(cx),
            _ => {},
        }
    }

    fn idle(&self) -> bool {
        self.source.state().pending == PairingPending::Idle
    }

    fn dismiss(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        if self.idle() {
            cx.emit(Event::Close);
        }
    }

    pub(crate) fn advance(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        if !self.idle() {
            return;
        }
        match self.source.state().stage {
            PairingStage::Address => self.source.act(Action::SubmitAddress {
                name: stage_widgets::text_of(&self.name, cx),
                address: stage_widgets::text_of(&self.address, cx),
            }),
            PairingStage::Fingerprint => self.source.act(Action::Confirm),
            PairingStage::Code => {
                let code = stage_widgets::text_of(&self.code, cx);
                self.source.act(Action::SubmitCode(code));
            },
            PairingStage::Paired => cx.emit(Event::Close),
        }
    }

    pub(crate) fn step_back(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        if !self.idle() {
            return;
        }
        match self.source.state().stage {
            PairingStage::Address | PairingStage::Paired => cx.emit(Event::Close),
            PairingStage::Fingerprint | PairingStage::Code => self.source.act(Action::Back),
        }
    }

    fn stage_body(
        &self,
        state: &PairingState,
        (palette, mono): (&Palette, &'static str),
    ) -> Div {
        let body = div().flex().flex_col().gap(GAP);
        match state.stage {
            PairingStage::Address => body
                .child(stage_widgets::labelled("Name", palette, &self.name))
                .child(stage_widgets::labelled("Address", palette, &self.address)),
            PairingStage::Fingerprint => body.child(stage_widgets::fingerprint_box(
                palette,
                &state.fingerprint,
                mono,
            )),
            PairingStage::Code => body.child(stage_widgets::labelled("Code", palette, &self.code)),
            PairingStage::Paired => body,
        }
    }
}

impl Render for View {
    fn render(
        &mut self,
        _window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) -> impl IntoElement {
        let state = self.source.state();
        let palette = *Theme::of(cx).palette(Surface::Window);
        let me = cx.entity();
        let body = self
            .stage_body(&state, (&palette, Theme::of(cx).mono_family()))
            .child(stage_widgets::muted(&palette).child(stage_copy::lead(state.stage)))
            .children(stage_widgets::messages(&state, &palette));
        Dialog::new("pairing")
            .title(TITLE)
            .size(DialogSize::Sm)
            .body(body)
            .footer(stage_widgets::footer(
                &state,
                (
                    cx.listener(|this, _, _, cx| this.advance(cx)),
                    cx.listener(|this, _, _, cx| this.step_back(cx)),
                ),
            ))
            .on_close(move |_, cx| me.update(cx, Self::dismiss))
    }
}

#[cfg(test)]
mod tests {
    use std::cell::RefCell;

    use gpui::{TestAppContext, VisualTestContext};

    use super::*;

    #[derive(Default)]
    struct Fake {
        state: RefCell<PairingState>,
        calls: RefCell<Vec<String>>,
    }

    impl Fake {
        fn note(
            &self,
            call: String,
        ) {
            self.calls.borrow_mut().push(call);
        }
    }

    fn address_call(
        name: &str,
        address: &str,
    ) -> String {
        format!("address {name} {address}")
    }

    impl Source for Fake {
        fn state(&self) -> PairingState {
            self.state.borrow().clone()
        }

        fn act(
            &self,
            action: Action,
        ) {
            let call = match action {
                Action::Edited => "edited".to_owned(),
                Action::SubmitAddress { name, address } => address_call(&name, &address),
                Action::Confirm => "confirm".to_owned(),
                Action::Back => "back".to_owned(),
                Action::SubmitCode(code) => format!("code {code}"),
            };
            self.note(call);
        }
    }

    type Events = Rc<RefCell<Vec<Event>>>;

    fn record(
        view: &Entity<View>,
        sink: Events,
        cx: &mut gpui::App,
    ) {
        cx.subscribe(view, move |_, event: &Event, _| {
            sink.borrow_mut().push(*event);
        })
        .detach();
    }

    fn open<'a>(
        cx: &'a mut TestAppContext,
        fake: &Rc<Fake>,
        events: &Events,
    ) -> (Entity<View>, &'a mut VisualTestContext) {
        let source = Rc::clone(fake);
        let (view, cx) = cx.add_window_view(|window, cx| View::over(source, window, cx));
        let sink = Rc::clone(events);
        cx.update(|_, cx| record(&view, sink, cx));
        (view, cx)
    }

    fn at(
        fake: &Fake,
        stage: PairingStage,
        pending: PairingPending,
    ) {
        let mut state = fake.state.borrow_mut();
        state.stage = stage;
        state.pending = pending;
    }

    fn each_stage_sends_its_step_to_the_session(cx: &mut TestAppContext) {
        let (fake, events) = (Rc::new(Fake::default()), Events::default());
        let (view, cx) = open(cx, &fake, &events);
        view.update(cx, |this, cx| {
            this.name.update(cx, |input, cx| input.set_text("Lab", cx));
            this.address
                .update(cx, |input, cx| input.set_text("box:3737", cx));
            this.advance(cx);
        });
        at(&fake, PairingStage::Fingerprint, PairingPending::Idle);
        view.update(cx, View::advance);
        at(&fake, PairingStage::Code, PairingPending::Idle);
        view.update(cx, |this, cx| {
            this.code
                .update(cx, |input, cx| input.set_text("123456", cx));
            this.advance(cx);
            this.step_back(cx);
        });
        assert_eq!(
            *fake.calls.borrow(),
            ["address Lab box:3737", "confirm", "code 123456", "back"]
        );
    }

    fn nothing_dismisses_the_dialog_while_the_host_answers(cx: &mut TestAppContext) {
        let (fake, events) = (Rc::new(Fake::default()), Events::default());
        fake.calls.borrow_mut().clear();
        let (view, cx) = open(cx, &fake, &events);
        at(&fake, PairingStage::Code, PairingPending::Pairing);
        view.update(cx, |this, cx| {
            this.dismiss(cx);
            this.step_back(cx);
            this.advance(cx);
        });
        assert!(events.borrow().is_empty() && fake.calls.borrow().is_empty());
        at(&fake, PairingStage::Address, PairingPending::Idle);
        view.update(cx, View::step_back);
        assert_eq!(*events.borrow(), [Event::Close]);
    }

    fn a_finished_pairing_is_announced_once_and_done_closes(cx: &mut TestAppContext) {
        let (fake, events) = (Rc::new(Fake::default()), Events::default());
        let (view, cx) = open(cx, &fake, &events);
        at(&fake, PairingStage::Paired, PairingPending::Idle);
        view.update(cx, View::changed);
        view.update(cx, View::changed);
        view.update(cx, View::advance);
        assert_eq!(*events.borrow(), [Event::Paired, Event::Close]);
    }

    const CASES: [fn(&mut TestAppContext); 3] = [
        each_stage_sends_its_step_to_the_session,
        nothing_dismisses_the_dialog_while_the_host_answers,
        a_finished_pairing_is_announced_once_and_done_closes,
    ];

    #[gpui::test]
    fn every_step_ends_as_the_session_allows(cx: &mut TestAppContext) {
        cx.update(|cx| ui::init(cx).unwrap());
        for case in CASES {
            case(cx);
        }
    }
}
