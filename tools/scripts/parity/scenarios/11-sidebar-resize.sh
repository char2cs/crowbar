# Resize handle, drag-collapse, toggle and peek. Start: sidebar open at the default 294px.
# Spec numbers (sidebar.md): default 294, min 250, max 640, collapse when dragged below 125.
move 294 400; shot sbr-01-handle-hover "resize handle hover (cursor not captured)"
drag 294 400 420 400 15; shot sbr-02-wide "dragged to 420"
drag 420 400 200 400 15; shot sbr-03-min "dragged below min: clamped to 250"
drag 250 400 100 400 15; shot sbr-04-drag-collapsed "dragged below 125: collapsed"
move 30 28; shot sbr-05-toggle-hover-collapsed "collapsed, pointer on toggle (peek opens)"
settle 0.6; shot sbr-06-peek-open "peek overlay settled"
move 700 400; settle 0.8; shot sbr-07-peek-closed "pointer left: peek gone"
click 30 28; settle 0.9; shot sbr-08-reopened "toggle click reopens at stored width (250)"
record sbr-anim-collapse 1 30 -- click 238 21
record sbr-anim-expand 1 30 -- click 30 28
