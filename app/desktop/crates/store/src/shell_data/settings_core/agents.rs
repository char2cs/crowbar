//! The Agents page: edits, records, artwork, descriptors and their reports and findings, and permission settings.

mod artwork;
mod descriptor_finding;
pub(crate) mod descriptor_records;
pub(crate) mod descriptor_report;
pub(crate) mod descriptors;
mod edit;
pub(crate) mod permission_key;
mod permission_setting;
pub(crate) mod records;

pub(crate) use permission_key::{PERMISSION, PERMISSION_PATH};
