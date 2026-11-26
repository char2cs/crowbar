//! JavaScript injected into the page: agent operations, eval and element picking.

use serde_json::Value;

const REPORT: &str = include_str!("script/agent_report.js");
const PAGE_AGENT: &str = include_str!("../../../panes/src/features/browser/lib/page-agent.js");
const POINTER_AGENT: &str =
    include_str!("../../../panes/src/features/browser/lib/pointer-agent.js");

/// The script that runs one DOM op of the page scripts (`snapshot`, `click`, `type`, `press_key`,
/// `scroll`, `wait`, `resolve`) for an agent. The page answers the request `id` through
/// `window.ipc.postMessage` with the op's result or its failure.
#[must_use]
pub fn agent_script(
    id: &str,
    op: &str,
    args: &Value,
) -> String {
    reporting(id, &op_body(op, &args.to_string()))
}

/// The script that runs `js`, the body of an async function, for an agent and reports its
/// return value, or its failure, for the request `id`.
#[must_use]
pub fn eval_script(
    id: &str,
    js: &str,
) -> String {
    reporting(id, js)
}

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

fn reporting(
    id: &str,
    body: &str,
) -> String {
    let id = Value::from(id);
    format!("({REPORT})({id}, async () => {{ {body}\n}});")
}

fn op_body(
    op: &str,
    args: &str,
) -> String {
    let op = Value::from(op);
    format!(
        "const base = ({PAGE_AGENT})(); const run = ({POINTER_AGENT})(base); return run({op}, {args});"
    )
}

#[cfg(test)]
mod tests {
    use serde_json::json;

    use super::*;

    #[test]
    fn a_script_runs_its_code_and_answers_for_its_request_only() {
        let click = agent_script("12", "click", &json!({ "ref": "e3" }));
        let hostile = eval_script("a\"};alert(1);//", "return 1");
        let cases = [
            (click.clone(), r#"run("click", {"ref":"e3"})"#),
            (click.clone(), r#")("12", async () => {"#),
            (click.clone(), "function pageAgent"),
            (click, "function pointerAgent"),
            (
                eval_script("3", "return 1 + 1"),
                "async () => { return 1 + 1\n});",
            ),
            (hostile, r#")("a\"};alert(1);//", async"#),
            (pick_script(), r#"run("pick", {})"#),
            (pick_script(), "crowbarPick"),
            (cancel_pick_script(), r#"run("cancelPick", {})"#),
        ];
        for (script, expected) in cases {
            assert!(script.contains(expected), "{expected}: {script}");
        }
        assert!(
            !cancel_pick_script().contains("postMessage"),
            "a cancel does not report back"
        );
    }
}
