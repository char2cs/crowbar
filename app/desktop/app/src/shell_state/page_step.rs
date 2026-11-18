use browser::{agent_script, eval_script};
use browser_agent::{AgentCommand, AgentOp};
use pane_host::BrowseAction;

#[derive(Debug, PartialEq)]
pub(crate) enum PageStep {
    Browse(BrowseAction, &'static str),
    Script(String),
    Shot,
    Refused(String),
}

impl PageStep {
    pub(crate) fn of(command: &AgentCommand) -> Self {
        match &command.op {
            AgentOp::Back => Self::Browse(BrowseAction::Back, "went back"),
            AgentOp::Forward => Self::Browse(BrowseAction::Forward, "went forward"),
            AgentOp::Reload => Self::Browse(BrowseAction::Reload, "reloading"),
            AgentOp::Screenshot => Self::Shot,
            AgentOp::Eval { js } => Self::Script(eval_script(&command.id, js)),
            AgentOp::Page { op, args } => Self::Script(agent_script(&command.id, op.name(), args)),
            other => Self::Refused(format!("{other:?} is not a page operation")),
        }
    }
}

#[cfg(test)]
mod tests {
    use browser_agent::DomOp;
    use serde_json::json;

    use super::*;

    fn command(op: AgentOp) -> AgentCommand {
        AgentCommand {
            id: "7".into(),
            chat_id: "c1".into(),
            tab: "b1".into(),
            op,
        }
    }

    #[test]
    fn each_page_operation_becomes_the_step_that_carries_it_out() {
        let eval = PageStep::of(&command(AgentOp::Eval {
            js: "return 1".into(),
        }));
        let click = PageStep::of(&command(AgentOp::Page {
            op: DomOp::Click,
            args: json!({ "ref": "e3" }),
        }));
        let cases = [
            (
                AgentOp::Back,
                PageStep::Browse(BrowseAction::Back, "went back"),
            ),
            (
                AgentOp::Forward,
                PageStep::Browse(BrowseAction::Forward, "went forward"),
            ),
            (
                AgentOp::Reload,
                PageStep::Browse(BrowseAction::Reload, "reloading"),
            ),
            (AgentOp::Screenshot, PageStep::Shot),
        ];
        for (op, step) in cases {
            assert_eq!(PageStep::of(&command(op)), step);
        }
        let scripts = [(eval, "return 1"), (click, "e3")];
        for (step, wanted) in scripts {
            assert!(
                matches!(&step, PageStep::Script(js) if js.contains(wanted) && js.contains('7')),
                "{step:?}"
            );
        }
        assert!(matches!(
            PageStep::of(&command(AgentOp::Close)),
            PageStep::Refused(_)
        ));
    }
}
