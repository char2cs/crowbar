/// Whether `text` is the bootstrap script's pointer report, and what it says.
pub(crate) fn parse_hover(text: &str) -> Option<bool> {
    let value: serde_json::Value = serde_json::from_str(text).ok()?;
    if value.get("kind")?.as_str()? != "shell.hover" {
        return None;
    }
    value.get("hovered")?.as_bool()
}

#[cfg(test)]
#[path = "hover_message_tests.rs"]
mod tests;
