//! A button between blocks, such as Add host.

use gpui::prelude::FluentBuilder as _;
use gpui::{IntoElement, ParentElement, Styled, div};
use settings::{Action, ButtonSpec};
use ui::{Button, ButtonSize, ButtonVariant};

use crate::frame::Frame;
use crate::frame::element_id;
use crate::frame::icon_named;
use crate::frame::spacing;
use crate::intent::Intent;

pub(crate) fn button_block(
    frame: &Frame<'_>,
    spec: &ButtonSpec,
) -> Option<impl IntoElement> {
    if matches!(spec.action(), Action::ResetOverrides(_)) && !frame.state.has_custom_chords() {
        return None;
    }
    let button = Button::new(element_id("block", spec.label()), spec.label().to_owned())
        .variant(ButtonVariant::Outline)
        .size(ButtonSize::Sm)
        .on_click(frame.sink.send(Intent::Button(spec.clone())));
    let button = match spec.icon_stem().and_then(icon_named) {
        Some(icon) => button.icon(icon),
        None => button,
    };
    let button = button.when(!frame.state.button_enabled(spec), Button::disabled);
    Some(div().px(spacing::TIGHT).flex().child(button))
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;
    use settings::{Block, default_schema};

    use crate::intent::Intent;
    use crate::test_support::{open, run};

    #[gpui::test]
    fn the_add_host_button_runs_its_backend_action(cx: &mut TestAppContext) {
        let (harness, cx) = open(cx, "hosts");
        let schema = default_schema().unwrap();
        let spec = schema
            .pages()
            .iter()
            .flat_map(|page| page.blocks().iter())
            .find_map(|block| match block {
                Block::Button(spec) if spec.label() == "Add host" => Some(spec.clone()),
                _ => None,
            })
            .unwrap();
        run(&harness, Intent::Button(spec), cx);
        assert_eq!(harness.backend.invoked(), ["hosts.add"]);
    }

    #[gpui::test]
    fn reset_all_clears_every_custom_chord_without_asking(cx: &mut TestAppContext) {
        use chord::Primary;
        use settings::{KEYBINDING_OVERRIDES, Key, ModifierKey, Pressed, Read as _, Value};

        let (harness, cx) = open(cx, "keybindings");
        run(
            &harness,
            Intent::Capture("navigation.toggleConsole".to_owned()),
            cx,
        );
        run(
            &harness,
            Intent::Key(Pressed::new(
                "k",
                &[ModifierKey::primary(Primary::native())],
            )),
            cx,
        );
        let schema = default_schema().unwrap();
        let spec = schema
            .pages()
            .iter()
            .flat_map(|page| page.blocks().iter())
            .find_map(|block| match block {
                Block::Button(spec) if spec.label().starts_with("Reset") => Some(spec.clone()),
                _ => None,
            })
            .unwrap();
        run(&harness, Intent::Button(spec), cx);
        assert!(
            matches!(harness.backend.get(&Key::new(KEYBINDING_OVERRIDES)), Some(Value::Map(map)) if map.is_empty())
        );
    }
}
