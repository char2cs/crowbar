use super::*;

#[test]
fn every_state_maps_to_its_tone_and_label() {
    let cases = [
        (HostState::Up, HostTone::Online, "Online"),
        (
            HostState::NoLiveStream,
            HostTone::Online,
            "Online — no live stream",
        ),
        (HostState::Down, HostTone::Offline, "Offline"),
        (
            HostState::NotAvailable,
            HostTone::Offline,
            "Host not available",
        ),
        (
            HostState::NeedsRepair,
            HostTone::NeedsAttention,
            "Needs pairing again",
        ),
        (HostState::Unknown, HostTone::Unsettled, "Status unknown"),
        (HostState::Probing, HostTone::Unsettled, "Checking…"),
    ];
    for (state, tone, label) in cases {
        assert_eq!(state.tone(), tone, "{state:?} tone");
        assert_eq!(state.label(), label, "{state:?} label");
    }
}
