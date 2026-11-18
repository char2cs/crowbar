use crate::script::page_calls::op_body;

/// The script that starts a pick in the page. It posts the finished pick, or the failure, with
/// `window.ipc.postMessage`, which is the only way the page answers.
#[must_use]
pub fn pick_script() -> String {
    let body = op_body("pick", "{}");
    format!(
        "(async () => {{ {body} }})().then(\
         (result) => window.ipc.postMessage(JSON.stringify({{ crowbarPick: {{ result }} }})), \
         (error) => window.ipc.postMessage(JSON.stringify({{ crowbarPick: {{ error: String(error && error.message || error) }} }})));"
    )
}

/// The script that ends a pick in progress; the pending pick then posts a null result.
#[must_use]
pub fn cancel_pick_script() -> String {
    let body = op_body("cancelPick", "{}");
    format!("(async () => {{ {body} }})().catch(() => undefined);")
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn the_scripts_run_their_own_op_and_only_a_pick_reports_back() {
        let pick = pick_script();
        let cancel = cancel_pick_script();
        assert!(pick.contains(r#"run("pick", {})"#));
        assert!(pick.contains("crowbarPick"));
        assert!(cancel.contains(r#"run("cancelPick", {})"#));
        assert!(!cancel.contains("postMessage"));
    }
}
