//! Button rows and the confirmation prompt in front of them.

use super::SettingsState;
use crate::action::Action;
use crate::button_spec::ButtonSpec;
use crate::confirm::Confirm;
use crate::effect::Effect;
use crate::prompt::Prompt;

impl SettingsState {
    /// Whether the button `spec` can be pressed.
    pub fn button_enabled(
        &self,
        spec: &ButtonSpec,
    ) -> bool {
        spec.enabled
            .as_ref()
            .is_none_or(|rule| rule.holds(&self.scope()))
    }

    /// Presses `spec`: asks its confirmation first, or returns the effect at once.
    pub fn press_button(
        &mut self,
        spec: &ButtonSpec,
    ) -> Option<Effect> {
        if !self.button_enabled(spec) {
            return None;
        }
        let effect = match &spec.action {
            Action::Backend(id) => Effect::Invoke(id.clone()),
            Action::ResetOverrides(key) => self.clear_overrides(key)?,
        };
        self.confirm_or_run(spec.confirm.as_ref(), effect)
    }

    /// Runs `effect` after asking `confirm`, or at once when there is nothing to ask.
    pub fn confirm_or_run(
        &mut self,
        confirm: Option<&Confirm>,
        effect: Effect,
    ) -> Option<Effect> {
        let Some(confirm) = confirm else {
            return Some(effect);
        };
        self.prompt = Some(Prompt {
            confirm: confirm.clone(),
            then: effect,
        });
        None
    }

    /// The question waiting for an answer.
    pub fn prompt(&self) -> Option<&Prompt> {
        self.prompt.as_ref()
    }

    /// The user confirmed: the effect the question guarded.
    pub fn accept_prompt(&mut self) -> Option<Effect> {
        self.prompt.take().map(|prompt| prompt.then)
    }

    /// The user declined.
    pub fn dismiss_prompt(&mut self) {
        self.prompt = None;
    }
}

#[cfg(test)]
mod tests {
    use crate::action::Action;
    use crate::button_spec::ButtonSpec;
    use crate::effect::Effect;
    use crate::prompt::Prompt;
    use crate::test_support::{clear_button, state, state_with};
    use crate::values::Values;

    #[test]
    fn buttons_follow_capabilities_and_confirm_before_running() {
        let mut state = state();
        let spec = clear_button();
        assert!(state.button_enabled(&spec));
        assert_eq!(
            state.press_button(&spec),
            None,
            "the confirmation comes first"
        );
        assert_eq!(state.prompt().map(Prompt::title), Some("Clear?"));
        assert_eq!(
            state.accept_prompt(),
            Some(Effect::Invoke("browser.clearData".to_owned()))
        );
        assert!(state.prompt().is_none());
        state.press_button(&spec);
        state.dismiss_prompt();
        assert!(state.prompt().is_none());
        let mut without = state_with(Values::default(), None);
        assert!(!without.button_enabled(&spec));
        assert_eq!(without.press_button(&spec), None);
        assert!(without.prompt().is_none(), "a disabled button asks nothing");
    }

    #[test]
    fn a_button_without_confirmation_returns_its_effect_at_once() {
        let mut state = state();
        let spec = ButtonSpec::new("Add host", Action::backend("hosts.add"));
        assert_eq!(
            state.press_button(&spec),
            Some(Effect::Invoke("hosts.add".to_owned()))
        );
    }
}
