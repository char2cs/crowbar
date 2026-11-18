//! The words of each stage of the dialog.

use shell_data::PairingStage;

pub(crate) fn lead(stage: PairingStage) -> &'static str {
    match stage {
        PairingStage::Address => "Name the host and say where it listens.",
        PairingStage::Fingerprint => {
            "Compare this fingerprint with the one `crowbar pair` shows on the host."
        },
        PairingStage::Code => "Enter the one-time code `crowbar pair` printed on the host.",
        PairingStage::Paired => "The host is paired and its spaces are loading.",
    }
}

pub(crate) fn primary(stage: PairingStage) -> &'static str {
    match stage {
        PairingStage::Address => "Continue",
        PairingStage::Fingerprint => "It matches",
        PairingStage::Code => "Pair",
        PairingStage::Paired => "Done",
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

    const CASES: [(PairingStage, &str, &str); 4] = [
        (PairingStage::Address, "Continue", "Cancel"),
        (PairingStage::Fingerprint, "It matches", "Back"),
        (PairingStage::Code, "Pair", "Back"),
        (PairingStage::Paired, "Done", "Cancel"),
    ];

    #[test]
    fn each_stage_names_its_buttons_and_says_what_to_do() {
        for (stage, primary_label, secondary_label) in CASES {
            assert_eq!(primary(stage), primary_label);
            assert_eq!(secondary(stage), secondary_label);
            assert_ne!(lead(stage), "");
        }
    }
}
