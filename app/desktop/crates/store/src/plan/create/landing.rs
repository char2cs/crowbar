//! Which face a new chat opens on, from the `chatIsDefaultPresentation` setting.

use settings::Value;
use sidebar::Command;

use crate::live::State;
use crate::live::Surface;

const CHAT_BY_DEFAULT: &str = "chatIsDefaultPresentation";

pub(crate) fn of(
    state: &State,
    command: &Command,
) -> Surface {
    surface_for(state.settings.get(CHAT_BY_DEFAULT), command)
}

fn surface_for(
    stored: Option<Value>,
    command: &Command,
) -> Surface {
    let chat_first = stored.and_then(|value| value.as_bool()).unwrap_or(true);
    match command {
        Command::NewTerminalThread(_) => Surface::Terminal,
        Command::NewThread(_) | Command::Thread(_) if !chat_first => Surface::Terminal,
        _ => Surface::Default,
    }
}

#[cfg(test)]
mod tests {
    use sidebar::{ProjectId, RowId};

    use super::*;

    #[test]
    fn plain_creates_follow_the_preference_and_a_terminal_thread_always_opens_on_its_terminal() {
        let new = Command::NewThread(ProjectId::from("p"));
        let thread = Command::Thread(RowId::from("r"));
        let terminal_thread = Command::NewTerminalThread(ProjectId::from("p"));
        let fork = Command::Fork {
            parent: RowId::from("r"),
            name: "f".into(),
        };
        let terminal_first = Some(Value::Bool(false));
        let chat_first = Some(Value::Bool(true));
        let cases = [
            (None, &new, Surface::Default),
            (chat_first.clone(), &new, Surface::Default),
            (terminal_first.clone(), &new, Surface::Terminal),
            (terminal_first.clone(), &thread, Surface::Terminal),
            (terminal_first.clone(), &fork, Surface::Default),
            (chat_first, &terminal_thread, Surface::Terminal),
            (terminal_first, &Command::OpenSettings, Surface::Default),
        ];
        for (stored, command, expected) in cases {
            assert_eq!(surface_for(stored.clone(), command), expected, "{stored:?}");
        }
    }
}
