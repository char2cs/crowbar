//! The widget of a row and how its value maps to text and back.

mod choice_source;
mod control_width;
mod issue;
mod legacy_number;
mod nudge;
mod number_range;
mod on_load;
mod text;
mod validator;

pub use choice_source::ChoiceSource;
pub use control_width::ControlWidth;
pub use issue::Issue;
pub use legacy_number::LegacyNumber;
pub use nudge::Nudge;
pub use number_range::NumberRange;
pub(crate) use on_load::OnLoad;
pub use validator::Validator;

use crate::schema::button_spec::ButtonSpec;

/// What a row shows on its right: the schema's fixed vocabulary of controls.
#[derive(Debug, Clone, PartialEq)]
pub enum Control {
    /// An on/off switch; `invert` flips it against the stored flag.
    Switch {
        /// Whether the switch reads opposite to the stored value.
        invert: bool,
    },
    /// A dropdown of options.
    Select {
        /// Where the options come from.
        source: ChoiceSource,
        /// Width class of the trigger.
        width: ControlWidth,
        /// Shown while nothing matches the stored value.
        placeholder: Option<String>,
        /// Tooltip text of an info icon after the trigger.
        hint: Option<String>,
    },
    /// A stepper with a typed field between minus and plus buttons.
    Number {
        /// Allowed numbers.
        range: NumberRange,
        /// Width class of the whole stepper.
        width: ControlWidth,
    },
    /// A single-line text field.
    Text {
        /// Shown while empty.
        placeholder: String,
        /// Width class.
        width: ControlWidth,
        /// Check that flags invalid text.
        validator: Option<Validator>,
    },
    /// A comma-separated list in a text field.
    TextList {
        /// Shown while empty.
        placeholder: String,
        /// Width class.
        width: ControlWidth,
    },
    /// A value shown as text, never edited.
    Readout {
        /// Whether the text uses the monospace family.
        mono: bool,
    },
    /// A button.
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
