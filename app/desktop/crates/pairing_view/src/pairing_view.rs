//! The dialog: name and address, fingerprint, then the one-time code.

use std::rc::Rc;
use std::sync::Arc;

use gpui::{
    AnyElement, AppContext as _, Context, Div, Entity, EventEmitter, IntoElement, ParentElement,
    Render, SharedString, Styled, Subscription, Window, div,
};
use shell_data::{PairingPending, PairingSession, PairingStage, PairingState};
use ui::{
    Button, ButtonSize, ButtonVariant, Dialog, DialogSize, Palette, Surface, TextInput,
    TextInputEvent, TextInputStyle, Theme,
};

use crate::look::{
    ADDRESS_PLACEHOLDER, CODE_PLACEHOLDER, FINGERPRINT_PAD, FINGERPRINT_TINT, GAP, LABEL_GAP,
    NAME_PLACEHOLDER, TEXT, TITLE,
};
use crate::pairing_event::PairingEvent;
use crate::pairing_source::PairingSource;
use crate::stage_copy;

const FIELDS: usize = 3;

/// The Add Host dialog; it emits a [`PairingEvent`] when the host is paired or it is dismissed.
pub struct PairingView {
    source: Rc<dyn PairingSource>,
    name: Entity<TextInput>,
    address: Entity<TextInput>,
    code: Entity<TextInput>,
    seen: PairingStage,
    _inputs: [Subscription; FIELDS],
}

impl std::fmt::Debug for PairingView {
    fn fmt(
        &self,
        formatter: &mut std::fmt::Formatter<'_>,
    ) -> std::fmt::Result {
        formatter
            .debug_struct("PairingView")
            .field("seen", &self.seen)
            .finish_non_exhaustive()
    }
}

impl EventEmitter<PairingEvent> for PairingView {}

fn field(
    placeholder: &'static str,
    cx: &mut Context<'_, PairingView>,
) -> Entity<TextInput> {
    cx.new(|cx| {
        TextInput::new(placeholder, cx)
            .on(Surface::Window)
            .style(TextInputStyle::Compact)
    })
}

fn muted(palette: &Palette) -> Div {
    div().text_size(TEXT).text_color(palette.muted_foreground())
}

fn labelled(
    label: &'static str,
    palette: &Palette,
    input: &Entity<TextInput>,
) -> Div {
    div()
        .flex()
        .flex_col()
        .gap(LABEL_GAP)
        .child(muted(palette).child(label))
        .child(input.clone())
}

fn fingerprint_box(
    palette: &Palette,
    fingerprint: &str,
    mono: &'static str,
) -> Div {
    div()
        .p(FINGERPRINT_PAD)
        .rounded_md()
        .bg(palette.accent().opacity(FINGERPRINT_TINT))
        .text_size(TEXT)
        .font_family(mono)
        .child(SharedString::from(fingerprint.to_owned()))
}

fn text_of(
    input: &Entity<TextInput>,
    cx: &Context<'_, PairingView>,
) -> String {
    input.read(cx).text().to_owned()
}

impl PairingView {
    /// A dialog over `session`, with the name field focused.
    pub fn new(
        session: Arc<PairingSession>,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) -> Self {
        Self::over(Rc::new(session), window, cx)
    }

    fn over(
        source: Rc<dyn PairingSource>,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) -> Self {
        let name = field(NAME_PLACEHOLDER, cx);
        let address = field(ADDRESS_PLACEHOLDER, cx);
        let code = field(CODE_PLACEHOLDER, cx);
        name.read(cx).focus(window);
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
            cx.emit(PairingEvent::Paired);
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
            TextInputEvent::Changed(_) => self.source.edited(),
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
            cx.emit(PairingEvent::Close);
        }
    }

    fn advance(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        if !self.idle() {
            return;
        }
        match self.source.state().stage {
            PairingStage::Address => self
                .source
                .submit_address(&text_of(&self.name, cx), &text_of(&self.address, cx)),
            PairingStage::Fingerprint => self.source.confirm(),
            PairingStage::Code => self.source.submit_code(&text_of(&self.code, cx)),
            PairingStage::Paired => cx.emit(PairingEvent::Close),
        }
    }

    fn step_back(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        if !self.idle() {
            return;
        }
        match self.source.state().stage {
            PairingStage::Address | PairingStage::Paired => cx.emit(PairingEvent::Close),
            PairingStage::Fingerprint | PairingStage::Code => self.source.back(),
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
                .child(labelled("Name", palette, &self.name))
                .child(labelled("Address", palette, &self.address)),
            PairingStage::Fingerprint => {
                body.child(fingerprint_box(palette, &state.fingerprint, mono))
            },
            PairingStage::Code => body.child(labelled("Code", palette, &self.code)),
            PairingStage::Paired => body,
        }
    }

    fn messages(
        state: &PairingState,
        palette: &Palette,
    ) -> Vec<AnyElement> {
        let notice = state
            .notice
            .as_ref()
            .map(|text| muted(palette).child(text.clone()).into_any_element());
        let error = state.error.as_ref().map(|text| {
            div()
                .text_size(TEXT)
                .text_color(palette.destructive())
                .child(text.clone())
                .into_any_element()
        });
        notice.into_iter().chain(error).collect()
    }

    fn footer(
        state: &PairingState,
        cx: &mut Context<'_, Self>,
    ) -> Div {
        let busy = state.pending != PairingPending::Idle;
        let mut primary = Button::new("pair-primary", stage_copy::primary(state.stage))
            .size(ButtonSize::Sm)
            .on_click(cx.listener(|this, _, _, cx| this.advance(cx)));
        let mut secondary = Button::new("pair-secondary", stage_copy::secondary(state.stage))
            .variant(ButtonVariant::Ghost)
            .size(ButtonSize::Sm)
            .on_click(cx.listener(|this, _, _, cx| this.step_back(cx)));
        if busy {
            primary = primary.disabled();
            secondary = secondary.disabled();
        }
        div().flex().gap(GAP).child(secondary).child(primary)
    }
}

impl Render for PairingView {
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
            .child(muted(&palette).child(stage_copy::lead(state.stage)))
            .children(Self::messages(&state, &palette));
        Dialog::new("pairing")
            .title(TITLE)
            .size(DialogSize::Sm)
            .body(body)
            .footer(Self::footer(&state, cx))
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

    impl PairingSource for Fake {
        fn state(&self) -> PairingState {
            self.state.borrow().clone()
        }

        fn edited(&self) {
            self.note("edited".to_owned());
        }

        fn submit_address(
            &self,
            name: &str,
            address: &str,
        ) {
            self.note(format!("address {name} {address}"));
        }

        fn confirm(&self) {
            self.note("confirm".to_owned());
        }

        fn back(&self) {
            self.note("back".to_owned());
        }

        fn submit_code(
            &self,
            code: &str,
        ) {
            self.note(format!("code {code}"));
        }
    }

    type Events = Rc<RefCell<Vec<PairingEvent>>>;

    fn record(
        view: &Entity<PairingView>,
        sink: Events,
        cx: &mut gpui::App,
    ) {
        cx.subscribe(view, move |_, event: &PairingEvent, _| {
            sink.borrow_mut().push(*event);
        })
        .detach();
    }

    fn open<'a>(
        cx: &'a mut TestAppContext,
        fake: &Rc<Fake>,
        events: &Events,
    ) -> (Entity<PairingView>, &'a mut VisualTestContext) {
        let source = Rc::clone(fake);
        let (view, cx) = cx.add_window_view(|window, cx| PairingView::over(source, window, cx));
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
        view.update(cx, PairingView::advance);
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
        view.update(cx, PairingView::step_back);
        assert_eq!(*events.borrow(), [PairingEvent::Close]);
    }

    fn a_finished_pairing_is_announced_once_and_done_closes(cx: &mut TestAppContext) {
        let (fake, events) = (Rc::new(Fake::default()), Events::default());
        let (view, cx) = open(cx, &fake, &events);
        at(&fake, PairingStage::Paired, PairingPending::Idle);
        view.update(cx, PairingView::changed);
        view.update(cx, PairingView::changed);
        view.update(cx, PairingView::advance);
        assert_eq!(
            *events.borrow(),
            [PairingEvent::Paired, PairingEvent::Close]
        );
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
