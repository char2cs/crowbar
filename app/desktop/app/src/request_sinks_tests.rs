use super::*;

#[test]
fn sidebar_commands_and_startup_buttons_reach_the_queue() {
    let (sender, receiver) = async_channel::unbounded();
    let sink = SidebarRequests(sender.clone());
    sidebar::CommandSink::dispatch(&sink, sidebar::Command::OpenSettings).unwrap();
    assert_eq!(
        receiver.try_recv().unwrap(),
        ShellRequest::Sidebar(sidebar::Command::OpenSettings),
        "sidebar"
    );
    drop(receiver);
    assert!(
        sidebar::CommandSink::dispatch(&sink, sidebar::Command::ImportRepo).is_err(),
        "a closed shell refuses"
    );
}
