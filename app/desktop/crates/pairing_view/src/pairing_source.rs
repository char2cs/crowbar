//! What the dialog needs of a pairing attempt.

use std::sync::Arc;

use shell_data::{PairingSession, PairingState};

pub(crate) trait PairingSource {
    fn state(&self) -> PairingState;
    fn edited(&self);
    fn submit_address(
        &self,
        name: &str,
        address: &str,
    );
    fn confirm(&self);
    fn back(&self);
    fn submit_code(
        &self,
        code: &str,
    );
}

impl PairingSource for Arc<PairingSession> {
    fn state(&self) -> PairingState {
        PairingSession::state(self)
    }

    fn edited(&self) {
        PairingSession::edited(self);
    }

    fn submit_address(
        &self,
        name: &str,
        address: &str,
    ) {
        PairingSession::submit_address(self, name, address);
    }

    fn confirm(&self) {
        PairingSession::confirm(self);
    }

    fn back(&self) {
        PairingSession::back(self);
    }

    fn submit_code(
        &self,
        code: &str,
    ) {
        PairingSession::submit_code(self, code);
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
