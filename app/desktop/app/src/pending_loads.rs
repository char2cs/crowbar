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

    pub(crate) fn take_closed(
        &mut self,
        is_open: impl Fn(&PaneId) -> bool,
    ) -> Vec<AgentCommand> {
        let gone: Vec<PaneId> = self
            .waiting
            .keys()
            .filter(|pane| !is_open(pane))
            .cloned()
            .collect();
        gone.iter().flat_map(|pane| self.take(pane)).collect()
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

    #[test]
    fn what_waited_for_a_pane_that_closed_is_handed_back_and_the_rest_stays() {
        let mut loads = PendingLoads::default();
        let (open, closed) = (PaneId::new("b1").unwrap(), PaneId::new("b2").unwrap());
        loads.defer(open.clone(), command("1"));
        loads.defer(closed, command("2"));
        let gone = loads.take_closed(|pane| *pane == open);
        assert_eq!(
            gone.iter().map(|c| c.id.as_str()).collect::<Vec<_>>(),
            ["2"]
        );
        assert_eq!(loads.take(&open).len(), 1, "the open pane keeps its wait");
    }
}
