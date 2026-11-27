mod choice_source;
mod control_width;
mod issue;
mod nudge;
mod number_range;
mod on_load;
mod text;
mod validator;

pub use choice_source::ChoiceSource;
pub use control_width::ControlWidth;
pub use issue::Issue;
pub use nudge::Nudge;
pub use number_range::NumberRange;
pub(crate) use on_load::OnLoad;
pub use validator::Validator;

use crate::schema::button_spec::ButtonSpec;

#[derive(Debug, Clone, PartialEq)]
pub enum Control {
    Switch {
        invert: bool,
    },
    Select {
        source: ChoiceSource,
        width: ControlWidth,
        placeholder: Option<String>,
        hint: Option<String>,
    },
    Number {
        range: NumberRange,
        width: ControlWidth,
    },
    Text {
        placeholder: String,
        width: ControlWidth,
        validator: Option<Validator>,
    },
    TextList {
        placeholder: String,
        width: ControlWidth,
    },
    Readout {
        mono: bool,
    },
    Button(ButtonSpec),
}

impl Control {
    pub(crate) fn switch() -> Self {
        Self::Switch { invert: false }
    }

    pub(crate) fn inverted_switch() -> Self {
        Self::Switch { invert: true }
    }

    pub(crate) fn select(
        options: &[(&str, &str)],
        width: ControlWidth,
    ) -> Self {
        Self::select_from(ChoiceSource::fixed(options), width)
    }

    pub(crate) fn provided_select(
        provider: &str,
        width: ControlWidth,
    ) -> Self {
        Self::select_from(ChoiceSource::provided(provider), width)
    }

    pub(crate) fn select_from(
        source: ChoiceSource,
        width: ControlWidth,
    ) -> Self {
        Self::Select {
            source,
            width,
            placeholder: None,
            hint: None,
        }
    }

    pub(crate) fn number(
        range: NumberRange,
        width: ControlWidth,
    ) -> Self {
        Self::Number { range, width }
    }

    pub(crate) fn text(
        placeholder: &str,
        width: ControlWidth,
    ) -> Self {
        Self::Text {
            placeholder: placeholder.to_owned(),
            width,
            validator: None,
        }
    }

    pub(crate) fn text_list(
        placeholder: &str,
        width: ControlWidth,
    ) -> Self {
        Self::TextList {
            placeholder: placeholder.to_owned(),
            width,
        }
    }

    #[must_use]
    pub(crate) fn validated(
        mut self,
        validator: Validator,
    ) -> Self {
        if let Self::Text {
            validator: slot, ..
        } = &mut self
        {
            *slot = Some(validator);
        }
        self
    }

    #[must_use]
    pub(crate) fn placeholder(
        mut self,
        text: &str,
    ) -> Self {
        if let Self::Select { placeholder, .. } = &mut self {
            *placeholder = Some(text.to_owned());
        }
        self
    }

    #[must_use]
    pub(crate) fn hint(
        mut self,
        text: &str,
    ) -> Self {
        if let Self::Select { hint, .. } = &mut self {
            *hint = Some(text.to_owned());
        }
        self
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn modifiers_apply_only_to_their_control() {
        let select = Control::provided_select("themes", ControlWidth::Wide)
            .placeholder("Select font...")
            .hint("Install a Nerd Font");
        assert!(matches!(
            select,
            Control::Select {
                placeholder: Some(_),
                hint: Some(_),
                ..
            }
        ));
        assert_eq!(
            Control::switch().placeholder("x").hint("y"),
            Control::switch()
        );
    }
}
