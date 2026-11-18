use serde_json::json;

use crate::selection::pick_report::PickReport;

const QUOTE_LABEL_CHARS: usize = 40;
const ELEMENT: &str = "element";
const DATA_VERSION: u8 = 1;

/// The text a pick leaves on the clipboard: readable lines, then one `⌖data` JSON line an agent
/// can parse. `tab` names the browser pane the pick came from.
#[must_use]
pub fn pointer_text(
    report: &PickReport,
    tab: &str,
) -> String {
    let mut lines = vec![headline(report, tab)];
    if report.kind != ELEMENT {
        lines.extend(
            report
                .items
                .iter()
                .enumerate()
                .map(|(index, item)| item_line(index, item)),
        );
    }
    lines.push(format!("⌖data {}", data_line(report, tab)));
    lines.join("\n")
}

/// How many things a pick found, the shown ones plus the rest.
#[must_use]
pub fn pick_count(report: &PickReport) -> usize {
    report.items.len() + report.more
}

fn label(report: &PickReport) -> String {
    if report.kind == "text" {
        return report
            .quote
            .as_deref()
            .unwrap_or_default()
            .chars()
            .take(QUOTE_LABEL_CHARS)
            .collect();
    }
    let Some(first) = report.items.first() else {
        return String::new();
    };
    [&first.name, &first.text]
        .into_iter()
        .find(|text| !text.is_empty())
        .cloned()
        .unwrap_or_default()
}

fn quoted(text: &str) -> String {
    serde_json::Value::String(text.to_owned()).to_string()
}

fn headline(
    report: &PickReport,
    tab: &str,
) -> String {
    let first = report.items.first();
    let kind = match (report.kind.as_str(), first) {
        (ELEMENT, Some(first)) if !first.role.is_empty() => first.role.clone(),
        (ELEMENT, Some(first)) => first.tag.clone(),
        (ELEMENT, None) => ELEMENT.to_owned(),
        (other, _) => other.to_owned(),
    };
    let count = pick_count(report);
    let tail = match (report.kind.as_str(), count) {
        (ELEMENT, _) => String::new(),
        (_, 1) => " · 1 item".to_owned(),
        (_, count) => format!(" · {count} items"),
    };
    format!(
        "⌖ {kind} {} · {} · tab {tab}{tail}",
        quoted(&label(report)),
        report.url
    )
}

fn item_line(
    index: usize,
    item: &crate::selection::pointer_item::PointerItem,
) -> String {
    let what = [&item.name, &item.text]
        .into_iter()
        .find(|text| !text.is_empty());
    let kind = [&item.role, &item.tag]
        .into_iter()
        .find(|name| !name.is_empty())
        .unwrap_or(&item.tag);
    let suffix = what.map_or_else(String::new, |what| format!(" {}", quoted(what)));
    format!(" {} {kind}{suffix}", index + 1)
}

fn data_line(
    report: &PickReport,
    tab: &str,
) -> String {
    let mut data = json!({
        "v": DATA_VERSION,
        "tab": tab,
        "url": report.url,
        "title": report.title,
        "kind": report.kind,
        "items": report.items,
        "more": report.more,
        "box": report.bounds,
    });
    if let (Some(quote), Some(map)) = (&report.quote, data.as_object_mut()) {
        map.insert("quote".to_owned(), json!(quote));
    }
    data.to_string()
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::selection::pointer_item::PointerItem;

    fn item(
        role: &str,
        name: &str,
    ) -> PointerItem {
        PointerItem {
            s: "#go".into(),
            tag: "button".into(),
            role: role.into(),
            name: name.into(),
            text: String::new(),
            r: vec![1, 2, 3, 4],
            h: "<button>".into(),
            i: 7,
        }
    }

    fn report(
        kind: &str,
        items: Vec<PointerItem>,
    ) -> PickReport {
        PickReport {
            kind: kind.into(),
            title: "T".into(),
            url: "https://a.test/".into(),
            items,
            more: 0,
            bounds: vec![1, 2, 3, 4],
            quote: None,
        }
    }

    #[test]
    fn a_pick_reads_as_a_headline_then_its_items_then_one_data_line() {
        let mut region = report("region", vec![item("", "A"), item("link", "")]);
        region.more = 3;
        let mut quote = report("text", vec![]);
        quote.quote = Some("x".repeat(60));
        let cases = [
            (
                report(ELEMENT, vec![item("button", "Go")]),
                vec!["⌖ button \"Go\" · https://a.test/ · tab p1"],
            ),
            (
                region,
                vec![
                    "⌖ region \"A\" · https://a.test/ · tab p1 · 5 items",
                    " 1 button \"A\"",
                    " 2 link",
                ],
            ),
            (
                quote,
                vec![
                    "⌖ text \"xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx\" · https://a.test/ · tab p1 · 0 items",
                ],
            ),
        ];
        for (found, expected) in cases {
            let text = pointer_text(&found, "p1");
            let lines: Vec<&str> = text.lines().collect();
            assert_eq!(lines[..lines.len() - 1], expected[..], "{text}");
            assert!(lines[lines.len() - 1].starts_with("⌖data {"), "{text}");
        }
    }

    #[test]
    fn the_data_line_names_the_tab_and_the_count_is_shown_plus_rest() {
        let mut found = report("multi", vec![item("", "A")]);
        found.more = 4;
        assert_eq!(pick_count(&found), 5);
        assert!(pointer_text(&found, "p9").contains("\"tab\":\"p9\""));
    }
}
