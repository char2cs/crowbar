use super::*;

#[test]
fn backend_actions_carry_their_id() {
    assert_eq!(
        Action::backend("hosts.add"),
        Action::Backend("hosts.add".to_owned())
    );
}
