use super::*;
use crate::demo_data::DemoData;

#[test]
fn the_active_project_is_one_the_data_lists() {
    let data = DemoData::rich();
    let active = data.active_project().expect("an active project");
    assert!(data.projects().iter().any(|project| project.id == active));
}

#[test]
fn a_loading_source_reports_pending_and_a_ready_one_ready() {
    assert_eq!(DemoData::loading().sync(), ScopeSync::Pending);
    assert_eq!(DemoData::reference().sync(), ScopeSync::Ready);
}
