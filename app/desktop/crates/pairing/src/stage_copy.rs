use store::{PairingPending, PairingStage, PairingState};

const ADDRESS_LEAD: &str = "On the host, open Settings → Devices and tap Add a device, or run crowbar pair: either shows the address, fingerprint and code you’ll need.";
const FINGERPRINT_LEAD: &str = "Confirm this matches the fingerprint shown on that host:";

pub(crate) fn lead(stage: PairingStage) -> Option<&'static str> {
    match stage {
        PairingStage::Address => Some(ADDRESS_LEAD),
        PairingStage::Fingerprint => Some(FINGERPRINT_LEAD),
        PairingStage::Code | PairingStage::Paired => None,
    }
}

pub(crate) fn primary(state: &PairingState) -> &'static str {
    match (state.pending, state.stage) {
        (PairingPending::Probing, _) => "Connecting…",
        (PairingPending::Pairing, _) => "Pairing…",
        (PairingPending::Idle, PairingStage::Address) => "Continue",
        (PairingPending::Idle, PairingStage::Fingerprint) => "Confirm fingerprint",
        (PairingPending::Idle, PairingStage::Code) => "Pair",
        (PairingPending::Idle, PairingStage::Paired) => "Done",
    }
}

pub(crate) fn secondary(stage: PairingStage) -> &'static str {
    match stage {
        PairingStage::Address | PairingStage::Paired => "Cancel",
        PairingStage::Fingerprint | PairingStage::Code => "Back",
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    type Case = (PairingStage, PairingPending, &'static str, &'static str);

    const CASES: [Case; 5] = [
        (
            PairingStage::Address,
            PairingPending::Idle,
            "Continue",
            "Cancel",
        ),
        (
            PairingStage::Address,
            PairingPending::Probing,
            "Connecting…",
            "Cancel",
        ),
        (
            PairingStage::Fingerprint,
            PairingPending::Idle,
            "Confirm fingerprint",
            "Back",
        ),
        (PairingStage::Code, PairingPending::Idle, "Pair", "Back"),
        (
            PairingStage::Code,
            PairingPending::Pairing,
            "Pairing…",
            "Back",
        ),
    ];

    fn state_at(
        stage: PairingStage,
        pending: PairingPending,
    ) -> PairingState {
        PairingState {
            stage,
            pending,
            ..PairingState::default()
        }
    }

    #[test]
    fn each_stage_names_its_buttons_in_develop_s_words() {
        for (stage, pending, primary_label, secondary_label) in CASES {
            assert_eq!(primary(&state_at(stage, pending)), primary_label);
            assert_eq!(secondary(stage), secondary_label);
        }
    }
}
