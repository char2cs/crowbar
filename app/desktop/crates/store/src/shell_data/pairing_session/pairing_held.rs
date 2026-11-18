//! What a pairing attempt keeps between its stages.

use crate::live::PairingRequest;
use crate::shell_data::pairing_session::host_id_text;
use crate::shell_data::pairing_session::pairing_pending::PairingPending;
use crate::shell_data::pairing_session::pairing_stage::PairingStage;
use crate::shell_data::pairing_session::pairing_state::PairingState;

#[derive(Debug, Default)]
pub(crate) struct PairingHeld {
    pub(crate) state: PairingState,
    pub(crate) host_id: String,
    pub(crate) dialed: String,
    pub(crate) name: String,
}

const CODE_DIGITS: usize = 6;

impl PairingHeld {
    pub(crate) fn code_shaped(code: &str) -> bool {
        code.len() == CODE_DIGITS && code.bytes().all(|byte| byte.is_ascii_digit())
    }

    pub(crate) fn step_back(&mut self) {
        let idle = self.state.pending == PairingPending::Idle;
        self.state.error = None;
        self.state.notice = None;
        self.state.stage = match self.state.stage {
            PairingStage::Code if idle => PairingStage::Fingerprint,
            PairingStage::Fingerprint if idle => PairingStage::Address,
            other => other,
        };
    }

    pub(crate) fn begin(
        &mut self,
        pending: PairingPending,
    ) -> bool {
        let idle = self.state.pending == PairingPending::Idle;
        if idle {
            self.state.pending = pending;
            self.state.error = None;
        }
        idle
    }

    pub(crate) fn enter_code(&mut self) {
        if self.state.stage == PairingStage::Fingerprint {
            self.state.stage = PairingStage::Code;
            self.state.error = None;
        }
    }

    pub(crate) fn confirm_next(
        &mut self,
        name: &str,
        endpoint: &str,
        fingerprint: String,
    ) {
        self.host_id = host_id_text::of(name);
        endpoint.clone_into(&mut self.dialed);
        name.clone_into(&mut self.name);
        self.state.fingerprint = fingerprint;
        self.state.stage = PairingStage::Fingerprint;
    }

    pub(crate) fn request(
        &self,
        code: &str,
    ) -> PairingRequest {
        PairingRequest {
            host_id: self.host_id.clone(),
            endpoint: self.dialed.clone(),
            fingerprint: self.state.fingerprint.clone(),
            code: code.to_owned(),
            name: self.name.clone(),
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn a_probe_is_held_until_the_next_confirm_and_a_request_carries_it() {
        let mut held = PairingHeld::default();
        assert!(held.begin(PairingPending::Probing));
        assert!(!held.begin(PairingPending::Probing));
        held.confirm_next("Lab", "box:3737", "ff".to_owned());
        held.enter_code();
        assert_eq!(held.state.stage, PairingStage::Code);
        let request = held.request("123456");
        assert_eq!(
            (
                request.endpoint.as_str(),
                request.name.as_str(),
                request.code.as_str()
            ),
            ("box:3737", "Lab", "123456")
        );
        assert_eq!(request.host_id, held.host_id);
        held.state.pending = PairingPending::Idle;
        held.step_back();
        assert_eq!(held.state.stage, PairingStage::Fingerprint);
        assert!(PairingHeld::code_shaped("123456") && !PairingHeld::code_shaped("12345a"));
    }
}
