use serde_json::Value;

use crate::script::page_calls::op_body;

const REPORT: &str = include_str!("../../../../panes/src/features/browser/lib/agent-report.js");

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

fn reporting(
    id: &str,
    body: &str,
) -> String {
    let id = Value::from(id);
    format!("({REPORT})({id}, async () => {{ {body}\n}});")
}

#[cfg(test)]
mod tests {
    use serde_json::json;

    use super::*;

    #[test]
    fn a_script_runs_its_code_and_answers_for_its_request_only() {
        let hostile = eval_script("a\"};alert(1);//", "return 1");
        let cases = [
            (
                "a dom op runs with its arguments",
                agent_script("12", "click", &json!({ "ref": "e3" })),
                r#"run("click", {"ref":"e3"})"#,
            ),
            (
                "a dom op answers for its request",
                agent_script("12", "click", &json!({})),
                r#")("12", async () => {"#,
            ),
            (
                "code runs as the body of an async function",
                eval_script("3", "return 1 + 1"),
                "async () => { return 1 + 1\n});",
            ),
            (
                "an id cannot break out of the script",
                hostile,
                r#")("a\"};alert(1);//", async"#,
            ),
        ];
        for (name, script, expected) in cases {
            assert!(script.contains(expected), "{name}: {script}");
        }
    }
}
