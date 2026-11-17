const PAGE_AGENT: &str = include_str!("../../../panes/src/features/browser/lib/page-agent.js");
const POINTER_AGENT: &str =
    include_str!("../../../panes/src/features/browser/lib/pointer-agent.js");

pub(crate) fn op_body(
    op: &str,
    args: &str,
) -> String {
    let op = serde_json::Value::from(op);
    format!(
        "const base = ({PAGE_AGENT})(); const run = ({POINTER_AGENT})(base); return run({op}, {args});"
    )
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn the_body_builds_the_page_scripts_and_runs_the_named_op() {
        let body = op_body("snapshot", "{}");
        assert!(body.ends_with(r#"return run("snapshot", {});"#), "{body}");
        assert!(
            body.contains("function pageAgent"),
            "the page script is inlined"
        );
        assert!(
            body.contains("function pointerAgent"),
            "the pointer script is inlined"
        );
    }
}
