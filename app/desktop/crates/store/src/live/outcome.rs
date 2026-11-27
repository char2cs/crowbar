use std::collections::BTreeMap;
use std::sync::Arc;

use sidebar::{RowId, WorkAtRisk};

#[derive(Debug, Clone, PartialEq, Eq)]
pub enum Outcome {
    OpenChat(String),
    OpenDraft(crate::live::DraftThread),
    PickDescriptor,
    PairHost,
    AddDevice,
    ClearBrowsingData,
    CopyText(String),
    NameRow {
        row: RowId,
        name: String,
    },
    NameAnswered {
        row: RowId,
        refusal: Option<String>,
    },
    Failed(String),
    Refused {
        title: String,
        reason: String,
    },
    Alert {
        title: String,
        message: String,
    },
    WorkAtRisk {
        row: RowId,
        at_risk: Vec<WorkAtRisk>,
    },
    ReviewViewed {
        workspace: String,
        viewed: BTreeMap<String, String>,
    },
    FileChanged {
        workspace: String,
        path: String,
    },
    FilesSynced {
        workspace: String,
    },
    RemovalRefused {
        row: RowId,
        reason: String,
    },
    Added(String),
    AlsoRemoved {
        titles: Vec<String>,
    },
}

pub type OutcomeSink = Arc<dyn Fn(Outcome) + Send + Sync>;
