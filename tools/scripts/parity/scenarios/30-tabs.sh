# Pane tab strip (per view) and view-tab strip (top bar). Start: restore golden snapshot.
# Golden layout: one view 'Untitled chat'; pane strip = chat, Branch Review (active), Terminal 2, Terminal 3.
shot tab-01-default "Branch Review active, 4 pane tabs"
move 690 65; shot tab-02-hover-idle "hover idle pane tab (Terminal 2)"
move 605 65; shot tab-03-hover-close "hover close x on active tab"
click 690 65; shot tab-04-terminal2-active "Terminal 2 active"
click 823 65; shot tab-05-terminal3-active "Terminal 3 active"
click 410 65; shot tab-06-chat-tab-active "chat tab active"
click 908 65; shot tab-07-plus-new-tab "plus: New Tab page tab"
rclick 690 65; shot_root tab-08-pane-tab-menu "right-click pane tab"; key Escape; settle
key ctrl+w; settle 0.5; shot tab-09-ctrl-w "ctrl+W closes the active tab"
key ctrl+shift+t; settle 0.6; shot tab-10-reopen "ctrl+shift+T reopens last closed"
click 105 230; settle 1.0; shot tab-20-second-view "sidebar chat row opens a second view tab"
move 664 21; shot tab-21-view-tab-hover "hover idle view tab"
rclick 664 21; shot_root tab-22-view-tab-menu "right-click view tab: native WebKit menu leaks (bug, see tabs-topbar.md)"; key Escape; settle
drag 664 21 360 21 14; shot tab-23-view-tab-reordered "second view tab dragged left (reorder)"
record tabm-pane-tab-switch 1 30 -- click 690 65
record tabm-view-tab-hover 1 30 -- move 424 21
