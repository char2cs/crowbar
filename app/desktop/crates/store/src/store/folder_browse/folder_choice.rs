use crate::live::State;
use crate::reads::{active_project, host_of};

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum FolderChoice {
    Native,
    OnHost,
}

pub(crate) fn of(state: &State) -> FolderChoice {
    let remote = active_project(state)
        .and_then(|project| host_of(state, project.as_str()))
        .is_some_and(|slot| slot.spec.endpoint.is_some());
    if remote {
        return FolderChoice::OnHost;
    }
    FolderChoice::Native
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::live::HostSpec;
    use crate::test_support::{state, state_on};

    #[test]
    fn a_space_on_this_computer_uses_the_native_dialog_and_one_on_a_paired_host_browses_its_disk() {
        let (local, _) = state();
        assert_eq!(of(&local), FolderChoice::Native);
        let spec = HostSpec {
            id: "box".into(),
            name: "Box".into(),
            endpoint: Some("box.lan:7000".into()),
        };
        let (remote, _) = state_on(spec);
        assert_eq!(of(&remote), FolderChoice::OnHost);
        assert_eq!(of(&State::new(Vec::new())), FolderChoice::Native);
    }
}
