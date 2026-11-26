//! The dialog: name and address, fingerprint, then the one-time code.

use std::rc::Rc;
use std::sync::Arc;

use gpui::{Context, Entity, EventEmitter, Subscription, Window};
use store::{PairingPending, PairingSession, PairingStage};
use ui::{TextInput, TextInputEvent};

use crate::action::Action;
use crate::code_slots::overrun;
use crate::event::Event;
use crate::look::{ADDRESS_PLACEHOLDER, NAME_PLACEHOLDER};
use crate::source::Source;
use crate::stage_widgets;

mod stage_render;

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
        let code = stage_widgets::field("", cx);
        name.update(cx, |field, cx| field.focus(window, cx));
        let inputs = [
            cx.subscribe(&name, Self::on_input),
            cx.subscribe(&address, Self::on_input),
            cx.subscribe(&code, Self::on_code),
        ];
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

    fn on_code(
        &mut self,
        input: Entity<TextInput>,
        event: &TextInputEvent,
        cx: &mut Context<'_, Self>,
    ) {
        if let TextInputEvent::Changed(typed) = event
            && let Some(kept) = overrun(typed)
        {
            input.update(cx, |field, cx| field.set_text(&kept, cx));
        }
        self.on_input(input, event, cx);
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

    // SPEC: https://github.com/char2cs/crowbar/blob/develop/web/src/features/hosts/components/add-host-dialog.tsx
    fn press_primary(
        &mut self,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        self.advance(cx);
        if self.source.state().stage == PairingStage::Code {
            self.code.update(cx, |field, cx| field.focus(window, cx));
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
}

#[cfg(test)]
mod tests {
    use std::cell::RefCell;

    use gpui::{
        AppContext as _, Div, Focusable as _, InteractiveElement as _, IntoElement, ParentElement,
        Render, Styled, TestAppContext, VisualTestContext, div,
    };
    use store::PairingState;

    use super::*;

    #[derive(Default)]
    struct Fake {
        state: RefCell<PairingState>,
        calls: RefCell<Vec<String>>,
    }

    impl Fake {
        fn confirmed(&self) -> String {
            self.state.borrow_mut().stage = PairingStage::Code;
            "confirm".to_owned()
        }

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
                Action::Confirm => self.confirmed(),
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

    type Opened<'a> = (Rc<Fake>, Events, Entity<View>, &'a mut VisualTestContext);

    fn open(cx: &mut TestAppContext) -> Opened<'_> {
        let (fake, events) = (Rc::new(Fake::default()), Events::default());
        let source = Rc::clone(&fake);
        let (view, cx) = cx.add_window_view(|window, cx| View::over(source, window, cx));
        let sink = Rc::clone(&events);
        cx.update(|_, cx| record(&view, sink, cx));
        (fake, events, view, cx)
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
        let (fake, _, view, cx) = open(cx);
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
        let (fake, events, view, cx) = open(cx);
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
        let (fake, events, view, cx) = open(cx);
        at(&fake, PairingStage::Paired, PairingPending::Idle);
        view.update(cx, View::changed);
        view.update(cx, View::changed);
        view.update(cx, View::advance);
        assert_eq!(*events.borrow(), [Event::Paired, Event::Close]);
    }

    fn confirming_the_fingerprint_puts_the_keyboard_in_the_code_field(cx: &mut TestAppContext) {
        let (fake, _, view, cx) = open(cx);
        at(&fake, PairingStage::Fingerprint, PairingPending::Idle);
        view.update_in(cx, View::press_primary);
        let on_code = view.update_in(cx, |this, window, cx| {
            this.code.focus_handle(cx).is_focused(window)
        });
        assert!(on_code, "the code field does not hold the keyboard");
    }

    fn a_seventh_character_of_the_code_is_refused(cx: &mut TestAppContext) {
        let (fake, _, view, cx) = open(cx);
        at(&fake, PairingStage::Fingerprint, PairingPending::Idle);
        view.update_in(cx, View::press_primary);
        at(&fake, PairingStage::Code, PairingPending::Idle);
        cx.simulate_input("1234567");
        let code = view.read_with(cx, |this, cx| this.code.read(cx).text().to_owned());
        assert_eq!(code, "123456");
    }

    const CASES: [fn(&mut TestAppContext); 5] = [
        each_stage_sends_its_step_to_the_session,
        nothing_dismisses_the_dialog_while_the_host_answers,
        a_finished_pairing_is_announced_once_and_done_closes,
        confirming_the_fingerprint_puts_the_keyboard_in_the_code_field,
        a_seventh_character_of_the_code_is_refused,
    ];

    struct Layers {
        held: gpui::FocusHandle,
        dialog: Option<Entity<View>>,
    }

    impl Render for Layers {
        fn render(
            &mut self,
            _: &mut Window,
            cx: &mut Context<'_, Self>,
        ) -> impl IntoElement {
            let me = cx.entity();
            div()
                .size_full()
                .child(ui::Portal::cover("layers", move |_, cx| shown(me.read(cx))))
        }
    }

    fn shown(layers: &Layers) -> Div {
        div()
            .size_full()
            .child(div().size(gpui::px(10.0)).track_focus(&layers.held))
            .children(layers.dialog.clone())
    }

    fn hold(
        layers: &Entity<Layers>,
        dialog: &Entity<View>,
        cx: &mut gpui::App,
    ) {
        layers.update(cx, |layers, cx| {
            layers.dialog = Some(dialog.clone());
            cx.notify();
        });
    }

    #[gpui::test]
    fn a_dialog_opened_while_a_field_holds_the_keyboard_types_into_its_name(
        cx: &mut TestAppContext
    ) {
        cx.update(|cx| ui::init(cx).unwrap());
        let window = cx.add_window(|_, cx| Layers {
            held: cx.focus_handle(),
            dialog: None,
        });
        let owner: gpui::AnyWindowHandle = window.into();
        let (mut owner, mut top) = (
            VisualTestContext::from_window(owner, cx),
            VisualTestContext::from_window(owner, cx),
        );
        let layers = window.entity(&owner).unwrap();
        layers.update_in(&mut owner, |layers, window, cx| {
            window.focus(&layers.held, cx);
            cx.notify();
        });
        owner.run_until_parked();
        let source: Rc<dyn Source> = Rc::new(Fake::default());
        let dialog = owner.update(|window, cx| cx.new(|cx| View::over(source, window, cx)));
        owner.update(|_, cx| hold(&layers, &dialog, cx));
        owner.run_until_parked();
        top.simulate_input("如同我");
        let name = dialog.read_with(&top, |view, cx| view.name.read(cx).text().to_owned());
        assert_eq!(name, "如同我", "the name field holds the keyboard");
    }

    #[gpui::test]
    fn every_step_ends_as_the_session_allows(cx: &mut TestAppContext) {
        cx.update(|cx| ui::init(cx).unwrap());
        for case in CASES {
            case(cx);
        }
    }
}
