//! The Agents page: edits, records, artwork, descriptors and their reports and findings, and permission settings.

mod artwork;
mod descriptor_finding;
pub(crate) mod descriptor_records;
pub(crate) mod descriptor_report;
pub(crate) mod descriptors;
mod edit;
mod permission_setting;
pub(crate) mod records;

pub(crate) const PERMISSION: &str = "defaultPermissionLevel";
pub(crate) const PERMISSION_PATH: &str = "/v0/settings/chat/permission-level";
