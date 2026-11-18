//! What the dialog needs of a pairing attempt.

use std::sync::Arc;

use store::{PairingSession, PairingState};

use crate::action::Action;

pub(crate) trait Source {
    fn state(&self) -> PairingState;
    fn act(
        &self,
        action: Action,
    );
}

impl Source for Arc<PairingSession> {
    fn state(&self) -> PairingState {
        PairingSession::state(self)
    }

    fn act(
        &self,
        action: Action,
    ) {
        match action {
            Action::Edited => PairingSession::edited(self),
            Action::SubmitAddress { name, address } => {
                PairingSession::submit_address(self, &name, &address);
            },
            Action::Confirm => PairingSession::confirm(self),
            Action::Back => PairingSession::back(self),
            Action::SubmitCode(code) => PairingSession::submit_code(self, &code),
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn needs_a_source<T: Source>() -> usize {
        size_of::<T>()
    }

    #[test]
    fn a_session_is_a_source() {
        assert!(needs_a_source::<Arc<PairingSession>>() > 0);
    }
}
