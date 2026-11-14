use ui::{ButtonVariant, IconName};

use super::*;

const ALL: [StartupCommand; 3] = [
    StartupCommand::Retry,
    StartupCommand::OpenLog,
    StartupCommand::Quit,
];

#[test]
fn every_command_has_a_distinct_label_and_id() {
    let labels: Vec<_> = ALL.iter().map(|command| command.label()).collect();
    let ids: Vec<_> = ALL.iter().map(|command| command.id()).collect();
    for list in [labels, ids] {
        let mut sorted = list.clone();
        sorted.sort_unstable();
        sorted.dedup();
        assert_eq!(sorted.len(), list.len(), "{list:?}");
    }
}

#[test]
fn only_the_primary_command_is_filled() {
    assert_eq!(
        StartupCommand::Quit.variant(StartupCommand::Quit),
        ButtonVariant::Primary
    );
    assert_eq!(
        StartupCommand::OpenLog.variant(StartupCommand::Quit),
        ButtonVariant::Outline
    );
    assert_eq!(StartupCommand::Retry.icon(), IconName::ArrowClockwise);
}
