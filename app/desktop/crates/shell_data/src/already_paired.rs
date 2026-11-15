//! Why a host cannot be paired a second time.

use crate::host_notes::HostNotes;
use crate::hub::Hub;

const REMEDY: &str = "Remove it in Settings → Hosts to pair it again.";

pub(crate) fn reason(
    (hub, notes): (&Hub, &HostNotes),
    (endpoint, fingerprint): (&str, &str),
) -> Option<String> {
    let at_address = hub.read(|state| {
        state
            .hosts
            .iter()
            .find(|slot| slot.spec.endpoint.as_deref() == Some(endpoint))
            .map(|slot| slot.spec.name.clone())
    });
    if let Some(name) = at_address {
        return Some(format!(
            "This address is already paired as {name}. {REMEDY}"
        ));
    }
    notes
        .all()
        .into_iter()
        .find(|(_, note)| !fingerprint.is_empty() && note.fingerprint == fingerprint)
        .map(|(_, note)| format!("This host is already paired as {}. {REMEDY}", note.name))
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::host_note::HostNote;
    use crate::test_support::state;

    #[test]
    fn a_known_address_or_a_known_fingerprint_is_already_paired() {
        let (mut state, _) = state();
        state.hosts[0].spec.endpoint = Some("box:3737".into());
        state.hosts[0].spec.name = "Lab".into();
        let hub = Hub::new(state, std::sync::Arc::new(|| {}));
        let dir = tempfile::tempdir().unwrap();
        let notes = HostNotes::open(dir.path().join("n.json")).unwrap();
        let note = HostNote {
            name: "Den".into(),
            client_id: "c".into(),
            fingerprint: "aa".into(),
        };
        notes.put("den", note).unwrap();
        let parts = (&*hub, &notes);
        let by_address = reason(parts, ("box:3737", "zz")).unwrap();
        assert!(by_address.contains("already paired as Lab"));
        let by_print = reason(parts, ("other:1", "aa")).unwrap();
        assert!(by_print.contains("already paired as Den"));
        assert_eq!(reason(parts, ("other:1", "bb")), None);
        assert_eq!(reason(parts, ("other:1", "")), None);
    }
}
