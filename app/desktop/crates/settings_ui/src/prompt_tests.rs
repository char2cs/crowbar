use super::*;

#[test]
fn exposes_the_wording() {
    let prompt = Prompt {
        confirm: Confirm::new("Kill?", "Ends the agent.", "Kill"),
        then: Effect::Invoke("kill".to_owned()),
    };
    assert_eq!(
        (prompt.title(), prompt.message(), prompt.confirm_label()),
        ("Kill?", "Ends the agent.", "Kill")
    );
}
