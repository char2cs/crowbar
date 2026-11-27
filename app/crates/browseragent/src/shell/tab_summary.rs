use serde::Serialize;

#[derive(Debug, Clone, PartialEq, Eq, Serialize)]
pub struct TabSummary {
    pub id: String,
    #[serde(rename = "chatId")]
    pub chat_id: String,
    pub url: String,
    pub title: String,
    pub loading: bool,
}
