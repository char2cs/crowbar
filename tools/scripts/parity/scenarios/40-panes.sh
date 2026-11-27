# Pane layout inside a view. Start: restore golden snapshot (one view, one pane).
shot pan-01-default "one pane: chat + editor tabs in one strip (right half < 780px)"
click 318 65; shot pan-02-strip-toggle "leading toggle (split/editor-area) clicked"
click 318 65; shot pan-03-strip-toggle-back "toggle again"
click 1178 65; shot pan-04-review-button "review shortcut button"
key ctrl+grave; settle 0.8; shot pan-10-console-open "console dock open (ctrl+grave)"
key ctrl+grave; settle 0.8; shot pan-11-console-closed "console closed (ctrl+grave again)"
dragstart 105 230; dragto 800 400; shot pan-20-drop-center "chat row held over pane centre: drop overlay"
dragto 330 400; shot pan-21-drop-left "held over the left edge zone"
dragto 1170 400; shot pan-22-drop-right "held over the right edge zone"
dragto 800 120; shot pan-23-drop-top "held over the top edge zone"
dragend; shot pan-24-after-drop "released: result of the drop"
record panm-console 1 30 -- key ctrl+grave
record panm-console-close 1 30 -- key ctrl+grave
