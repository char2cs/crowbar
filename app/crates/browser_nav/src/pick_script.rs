const PAGE_AGENT: &str = include_str!("../../../panes/src/features/browser/lib/page-agent.js");
const POINTER_AGENT: &str =
    include_str!("../../../panes/src/features/browser/lib/pointer-agent.js");

fn agent_call(op: &str) -> String {
    format!(
        "const base = ({PAGE_AGENT})(); const run = ({POINTER_AGENT})(base); return run('{op}', {{}});"
    )
}

/// The script that starts a pick in the page. It posts the finished pick, or the failure, with
/// `window.ipc.postMessage`, which is the only way the page answers.
#[must_use]
pub fn pick_script() -> String {
    let body = agent_call("pick");
    format!(
        "(async () => {{ {body} }})().then(\
         (result) => window.ipc.postMessage(JSON.stringify({{ crowbarPick: {{ result }} }})), \
         (error) => window.ipc.postMessage(JSON.stringify({{ crowbarPick: {{ error: String(error && error.message || error) }} }})));"
    )
}

/// The script that ends a pick in progress; the pending pick then posts a null result.
#[must_use]
pub fn cancel_pick_script() -> String {
    let body = agent_call("cancelPick");
    format!("(async () => {{ {body} }})().catch(() => undefined);")
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn the_scripts_run_their_own_op_and_only_a_pick_reports_back() {
        let pick = pick_script();
        let cancel = cancel_pick_script();
        assert!(pick.contains("run('pick', {})"));
        assert!(pick.contains("crowbarPick"));
        assert!(cancel.contains("run('cancelPick', {})"));
        assert!(!cancel.contains("postMessage"));
    }
}
