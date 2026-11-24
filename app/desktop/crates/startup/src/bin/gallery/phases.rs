//! The halt the startup gallery shows.

use startup::Input;

const HALT_MESSAGE: &str = "migrate: step 3 \"Convert workspace settings\": no such table: spaces";

pub(crate) fn halted() -> Input {
    Input::Halted {
        reason: Some("step_failed".to_owned()),
        message: HALT_MESSAGE.to_owned(),
    }
}
