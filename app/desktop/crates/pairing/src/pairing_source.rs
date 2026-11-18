//! What the dialog needs of a pairing attempt.

use std::sync::Arc;

use store::{PairingSession, PairingState};

use crate::pairing_action::PairingAction;

pub(crate) trait PairingSource {
    fn state(&self) -> PairingState;
    fn act(
        &self,
        action: PairingAction,
    );
}

impl PairingSource for Arc<PairingSession> {
    fn state(&self) -> PairingState {
        PairingSession::state(self)
    }

    fn act(
        &self,
        action: PairingAction,
    ) {
        match action {
            PairingAction::Edited => PairingSession::edited(self),
            PairingAction::SubmitAddress { name, address } => {
                PairingSession::submit_address(self, &name, &address);
            },
            PairingAction::Confirm => PairingSession::confirm(self),
            PairingAction::Back => PairingSession::back(self),
            PairingAction::SubmitCode(code) => PairingSession::submit_code(self, &code),
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn needs_a_source<T: PairingSource>() -> usize {
        size_of::<T>()
    }

    #[test]
    fn a_session_is_a_source() {
        assert!(needs_a_source::<Arc<PairingSession>>() > 0);
    }
}
