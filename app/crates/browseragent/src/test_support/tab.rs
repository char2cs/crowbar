use crate::shell::TabInfo;

pub(crate) fn tab(url: &str) -> TabInfo {
    TabInfo {
        id: "b1".into(),
        chat_id: "c1".into(),
        url: url.into(),
        title: String::new(),
        loading: false,
    }
}
