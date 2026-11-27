use gpui::{Context, Div, IntoElement, ParentElement, Render, Styled, Window, div};
use store::{PairingStage, PairingState};
use ui::{Dialog, DialogSize, Palette, Surface, Theme};

use crate::code_slots::CodeSlots;
use crate::look::{ADDRESS_HINT, CODE_HINT, GAP, TITLE};
use crate::stage_copy;
use crate::stage_widgets;
use crate::view::View;

impl View {
    fn stage_body(
        &self,
        state: &PairingState,
        (palette, mono): (&Palette, &'static str),
    ) -> Div {
        let body = div().flex().flex_col().gap(GAP);
        match state.stage {
            PairingStage::Address => body
                .child(stage_widgets::labelled("Name", palette, self.name.clone()))
                .child(stage_widgets::hinted(
                    stage_widgets::labelled("Address", palette, self.address.clone()),
                    ADDRESS_HINT,
                    palette,
                )),
            PairingStage::Fingerprint => body.child(stage_widgets::fingerprint_box(
                palette,
                &state.fingerprint,
                mono,
            )),
            PairingStage::Code => body.child(stage_widgets::hinted(
                stage_widgets::labelled(
                    "Pairing code",
                    palette,
                    CodeSlots {
                        input: self.code.clone(),
                    },
                ),
                CODE_HINT,
                palette,
            )),
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
        let lead =
            stage_copy::lead(state.stage).map(|lead| stage_widgets::muted(&palette).child(lead));
        let body = div()
            .flex()
            .flex_col()
            .gap(GAP)
            .children(lead)
            .child(self.stage_body(&state, (&palette, Theme::of(cx).mono_family())))
            .children(stage_widgets::messages(&state, &palette));
        Dialog::new("pairing")
            .title(TITLE)
            .size(DialogSize::Sm)
            .body(body)
            .footer(stage_widgets::footer(
                &state,
                (
                    cx.listener(|this, _, window, cx| this.press_primary(window, cx)),
                    cx.listener(|this, _, _, cx| this.step_back(cx)),
                ),
            ))
            .on_close(move |_, cx| me.update(cx, Self::dismiss))
    }
}

#[cfg(test)]
mod tests {
    use std::rc::Rc;

    use gpui::TestAppContext;

    use super::*;
    use crate::action::Action;
    use crate::source::Source;

    struct AtCode;

    impl Source for AtCode {
        fn state(&self) -> PairingState {
            PairingState {
                stage: PairingStage::Code,
                ..PairingState::default()
            }
        }

        fn act(
            &self,
            _: Action,
        ) {
        }
    }

    #[gpui::test]
    fn the_code_stage_draws_develops_slots_under_pairing_code(cx: &mut TestAppContext) {
        cx.update(|cx| ui::init(cx).unwrap());
        let (_, cx) = cx.add_window_view(|window, cx| View::over(Rc::new(AtCode), window, cx));
        cx.run_until_parked();
        let slots = cx.debug_bounds("pairing-code-slots").unwrap_or_default();
        let last = cx.debug_bounds("pairing-code-slot").unwrap_or_default();
        let after_five_and_the_rule = 5.0 * (32.0 + 8.0) + 12.0 + 8.0;
        assert_eq!(
            last.origin.x - slots.origin.x,
            gpui::px(after_five_and_the_rule)
        );
        assert_eq!(last.size, gpui::size(gpui::px(32.0), gpui::px(32.0)));
    }
}
