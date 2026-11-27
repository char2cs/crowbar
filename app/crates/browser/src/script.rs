use serde_json::Value;

const REPORT: &str = include_str!("script/agent_report.js");
const PAGE_AGENT: &str = include_str!("../../../panes/src/features/browser/lib/page-agent.js");
const POINTER_AGENT: &str =
    include_str!("../../../panes/src/features/browser/lib/pointer-agent.js");

#[must_use]
pub fn agent_script(
    id: &str,
    op: &str,
    args: &Value,
) -> String {
    reporting(id, &op_body(op, &args.to_string()))
}

#[must_use]
pub fn eval_script(
    id: &str,
    js: &str,
) -> String {
    reporting(id, js)
}

#[must_use]
pub fn pick_script() -> String {
    let body = op_body("pick", "{}");
    format!(
        "(async () => {{ {body} }})().then(\
         (result) => window.ipc.postMessage(JSON.stringify({{ crowbarPick: {{ result }} }})), \
         (error) => window.ipc.postMessage(JSON.stringify({{ crowbarPick: {{ error: String(error && error.message || error) }} }})));"
    )
}

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
