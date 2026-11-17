use std::collections::BTreeMap;

use browser_agent::AgentCommand;
use protocol::PaneId;

#[derive(Debug, Default)]
pub(crate) struct PendingLoads {
    waiting: BTreeMap<PaneId, Vec<AgentCommand>>,
}

impl PendingLoads {
    pub(crate) fn defer(
        &mut self,
        pane: PaneId,
        command: AgentCommand,
    ) {
        self.waiting.entry(pane).or_default().push(command);
    }

    pub(crate) fn take(
        &mut self,
        pane: &PaneId,
    ) -> Vec<AgentCommand> {
        self.waiting.remove(pane).unwrap_or_default()
    }
}

#[cfg(test)]
mod tests {
    use browser_agent::AgentOp;

    use super::*;

    fn command(id: &str) -> AgentCommand {
        AgentCommand {
            id: id.into(),
            chat_id: "c1".into(),
            tab: "b1".into(),
            op: AgentOp::Screenshot,
        }
    }

    #[test]
    fn what_waited_for_a_pane_runs_in_order_once_it_has_loaded() {
        let mut loads = PendingLoads::default();
        let (first, second) = (PaneId::new("b1").unwrap(), PaneId::new("b2").unwrap());
        loads.defer(first.clone(), command("1"));
        loads.defer(second.clone(), command("2"));
        loads.defer(first.clone(), command("3"));
        let ids =
            |commands: Vec<AgentCommand>| commands.into_iter().map(|c| c.id).collect::<Vec<_>>();
        assert_eq!(ids(loads.take(&first)), ["1", "3"]);
        assert_eq!(ids(loads.take(&first)), Vec::<String>::new(), "taken once");
        assert_eq!(ids(loads.take(&second)), ["2"]);
    }
}
