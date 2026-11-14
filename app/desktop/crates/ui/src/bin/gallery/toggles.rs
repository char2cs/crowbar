//! Switch, checkbox and the two tab styles.

use gpui::{Entity, IntoElement, ParentElement, Styled, div, px};
use ui::{Checkbox, IconName, Surface, Switch, TabItem, Tabs, TabsStyle};

use crate::page::Page;
use crate::section::row;

fn switches(page: &Page, entity: &Entity<Page>) -> impl IntoElement {
    let owner = entity.clone();
    let switch = Switch::new("switch", page.switch_on).on_change(move |value, _, cx| {
        owner.update(cx, |page, cx| page.edit(cx, |page| page.switch_on = value));
    });
    row()
        .gap(px(16.0))
        .child(switch)
        .child(Switch::new("switch-off", false))
        .child(Switch::new("switch-disabled", true).disabled())
}

fn checkboxes(page: &Page, entity: &Entity<Page>) -> impl IntoElement {
    let owner = entity.clone();
    let checkbox = Checkbox::new("check", page.checked).on_change(move |value, _, cx| {
        owner.update(cx, |page, cx| page.edit(cx, |page| page.checked = value));
    });
    row()
        .gap(px(16.0))
        .child(checkbox)
        .child(Checkbox::new("check-off", false))
        .child(Checkbox::new("check-disabled", true).disabled())
}

fn pill_tabs(page: &Page, entity: &Entity<Page>) -> impl IntoElement {
    let owner = entity.clone();
    let items = vec![
        TabItem::icon(IconName::ChatsCircle),
        TabItem::icon(IconName::FolderOpen),
        TabItem::icon(IconName::GitBranch),
    ];
    let tabs = Tabs::new("tabs", items, page.tab)
        .on(Surface::Window)
        .on_select(move |index, _, cx| {
            owner.update(cx, |page, cx| page.edit(cx, |page| page.tab = index));
        });
    div().w(px(294.0)).child(tabs)
}

fn segmented_tabs(page: &Page, entity: &Entity<Page>) -> impl IntoElement {
    let owner = entity.clone();
    let items = vec![TabItem::text("Chat"), TabItem::text("Terminal")];
    let tabs = Tabs::new("segments", items, page.segment)
        .style(TabsStyle::Segmented)
        .on_select(move |index, _, cx| {
            owner.update(cx, |page, cx| page.edit(cx, |page| page.segment = index));
        });
    div().w(px(200.0)).child(tabs)
}

pub(crate) fn toggles(page: &Page, entity: &Entity<Page>) -> impl IntoElement {
    div()
        .flex()
        .flex_col()
        .gap(px(12.0))
        .child(switches(page, entity))
        .child(checkboxes(page, entity))
        .child(pill_tabs(page, entity))
        .child(segmented_tabs(page, entity))
}
