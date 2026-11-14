//! Public-API tests: random command sequences never break an invariant, and undo is exact.

use model::{Branch, Command, Edge, Layout, LayoutHistory, PaneInstance, Placement, Tab, TabId};
use protocol::{PaneId, PaneState, PaneTypeId, Params};

struct Lcg(u64);

impl Lcg {
    fn next(&mut self, bound: usize) -> usize {
        self.0 = self
            .0
            .wrapping_mul(6_364_136_223_846_793_005)
            .wrapping_add(1_442_695_040_888_963_407);
        usize::try_from(self.0 >> 33).unwrap() % bound
    }
}

fn pane_ids(layout: &Layout) -> Vec<PaneId> {
    layout.tabs().iter().flat_map(Tab::pane_ids).collect()
}

fn pick(rng: &mut Lcg, ids: &[PaneId]) -> PaneId {
    ids.get(rng.next(ids.len().max(1)))
        .cloned()
        .unwrap_or_else(|| PaneId::new("missing").unwrap())
}

fn edge(rng: &mut Lcg) -> Edge {
    [Edge::Left, Edge::Right, Edge::Top, Edge::Bottom][rng.next(4)]
}

fn random_command(rng: &mut Lcg, layout: &Layout, serial: &mut u64) -> Command {
    let panes = pane_ids(layout);
    let tab = TabId(rng.next(4) as u64);
    *serial += 1;
    match rng.next(11) {
        0 => Command::AddTab {
            id: tab,
            title: "t".into(),
        },
        1 => Command::CloseTab { tab },
        2 => Command::ActivateTab { tab },
        3 => Command::MoveTab {
            tab,
            to_index: rng.next(4),
        },
        4 | 5 => Command::OpenPane {
            tab,
            instance: PaneInstance {
                pane_id: PaneId::new(format!("p{serial}")).unwrap(),
                type_id: PaneTypeId::new("core.chat").unwrap(),
                params: Params::default(),
                state: PaneState::default(),
            },
            placement: (rng.next(2) == 0).then(|| Placement {
                target: pick(rng, &panes),
                edge: edge(rng),
            }),
        },
        6 => Command::ClosePane {
            pane: pick(rng, &panes),
        },
        7 => Command::MovePane {
            pane: pick(rng, &panes),
            placement: Placement {
                target: pick(rng, &panes),
                edge: edge(rng),
            },
        },
        8 => Command::MovePaneToTab {
            pane: pick(rng, &panes),
            to: tab,
            placement: None,
        },
        9 => Command::SwapPanes {
            a: pick(rng, &panes),
            b: pick(rng, &panes),
        },
        _ => Command::ResizeSplit {
            tab,
            path: vec![Branch::First],
            ratio: rng.next(100) as f32 / 100.0,
        },
    }
}

#[test]
fn random_sequences_keep_every_invariant() {
    for seed in 0..40 {
        let mut rng = Lcg(seed);
        let mut serial = 0;
        let mut layout = Layout::default();
        for step in 0..300 {
            let command = random_command(&mut rng, &layout, &mut serial);
            let before = layout.clone();
            if layout.apply(command.clone()).is_err() {
                assert_eq!(
                    layout, before,
                    "seed {seed} step {step}: refused {command:?} changed the layout"
                );
            }
            layout
                .validate()
                .unwrap_or_else(|e| panic!("seed {seed} step {step} after {command:?}: {e}"));
        }
    }
}

#[test]
fn undoing_everything_returns_to_the_start() {
    let mut rng = Lcg(7);
    let mut serial = 0;
    let mut history = LayoutHistory::default();
    for _ in 0..60 {
        let command = random_command(&mut rng, history.layout(), &mut serial);
        let _refused = history.apply(command).is_err();
    }
    while history.undo() {}
    assert_eq!(history.layout(), &Layout::default(), "empty layout again");
    while history.redo() {}
    history.layout().validate().unwrap();
}

#[test]
fn panes_never_disappear_unless_closed() {
    let mut rng = Lcg(99);
    let mut serial = 0;
    let mut layout = Layout::default();
    for _ in 0..400 {
        let command = random_command(&mut rng, &layout, &mut serial);
        let removes = matches!(
            command,
            Command::ClosePane { .. } | Command::CloseTab { .. }
        );
        let before = pane_ids(&layout).len();
        let opens = matches!(command, Command::OpenPane { .. });
        if layout.apply(command).is_ok() && !removes {
            let after = pane_ids(&layout).len();
            assert_eq!(
                after,
                before + usize::from(opens),
                "pane count changed without open or close"
            );
        }
    }
}
