use crate::shell::TabSummary;

pub(crate) fn tab(url: &str) -> TabSummary {
    TabSummary {
        id: "b1".into(),
        chat_id: "c1".into(),
        url: url.into(),
        title: String::new(),
        loading: false,
    }
}
