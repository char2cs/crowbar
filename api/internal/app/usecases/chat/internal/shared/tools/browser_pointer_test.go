package tools_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/browserbridge"
)

func pointerLine(tab, url, kind string, items ...string) string {
	return fmt.Sprintf(`⌖data {"v":1,"tab":%q,"url":%q,"title":"Checkout","kind":%q,"items":[%s],"more":0}`,
		tab, url, kind, strings.Join(items, ","))
}

const headingItem = `{"s":"section.summary > h2","tag":"h2","role":"heading","name":"Order summary","text":"Order summary","r":[640,300,420,30],"h":"Checkout","i":0}`

func TestBrowserResolve_RendersFoundAndGone(t *testing.T) {
	w := newBrowserWorld(t)
	w.peer.reportTabs([2]string{"b-1", "chat-a"})
	w.peer.reply = `{"items":[{"found":true,"ref":"e7","matchedBy":"fingerprint","box":[1,2,3,4],"extra":1},{"found":false}]}`

	ptr := "⌖ region \"Order summary\"\n 1 h2\n" + pointerLine("b-1", "https://x.test", "region", headingItem, `{"tag":"p","text":"gone"}`)
	args, _ := json.Marshal(map[string]string{"pointer": "look at this:\n" + ptr + "\nthanks"})
	out, err := call(t, w.a, "browser_resolve", string(args))
	require.NoError(t, err)
	require.Contains(t, out, `1 h2 "Order summary": found e7 (fingerprint)`)
	require.Contains(t, out, `2 p "gone": gone`)
	require.NotContains(t, out, "navigated")

	ran := w.peer.ran()
	require.Len(t, ran, 1)
	require.Equal(t, "resolve", ran[0]["op"])
	require.Equal(t, "b-1", ran[0]["tab"])
	require.Equal(t, "chat-a", ran[0]["chatId"])
	require.JSONEq(t, `{"items":[`+headingItem+`,{"tag":"p","text":"gone"}],"detail":"refs"}`, mustJSON(ran[0]["args"]))
}

func TestBrowserResolve_DetailPassesThroughAndRenders(t *testing.T) {
	w := newBrowserWorld(t)
	w.peer.reportTabs([2]string{"b-1", "chat-a"})
	w.peer.reply = `{"items":[{"found":true,"ref":"e1","matchedBy":"selector","text":"Hello","html":"<h2>Hello</h2>","styles":{"color":"red"}}]}`

	for detail, want := range map[string]string{"text": "text: Hello", "html": "html: <h2>Hello</h2>", "styles": `styles: {"color":"red"}`} {
		args, _ := json.Marshal(map[string]string{"pointer": pointerLine("b-1", "https://x.test", "element", headingItem), "detail": detail})
		out, err := call(t, w.a, "browser_resolve", string(args))
		require.NoError(t, err)
		require.Contains(t, out, want)
		require.Contains(t, out, "found e1 (selector)")
		last := w.peer.ran()[len(w.peer.ran())-1]
		require.Contains(t, mustJSON(last["args"]), `"detail":"`+detail+`"`)
	}
}

func TestBrowserResolve_MultiplePointersEachResolve(t *testing.T) {
	w := newBrowserWorld(t)
	w.peer.reportTabs([2]string{"b-1", "chat-a"}, [2]string{"b-2", "chat-a"})
	w.peer.reply = `{"items":[{"found":true,"ref":"e3","matchedBy":"selector"}]}`
	text := pointerLine("b-1", "https://x.test", "element", headingItem) + "\njunk between\n" + pointerLine("b-2", "https://x.test", "element", headingItem) + "\r\n"
	args, _ := json.Marshal(map[string]string{"pointer": text})

	out, err := call(t, w.a, "browser_resolve", string(args))
	require.NoError(t, err)
	require.Contains(t, out, "pointer 1:")
	require.Contains(t, out, "pointer 2:")
	require.Len(t, w.peer.ran(), 2)
	require.Equal(t, "b-2", w.peer.ran()[1]["tab"])
}

func TestBrowserResolve_NoteWhenThePageNavigated(t *testing.T) {
	w := newBrowserWorld(t)
	w.peer.reportTabs([2]string{"b-1", "chat-a"}) // reports https://x.test
	w.peer.reply = `{"items":[{"found":false}]}`
	args, _ := json.Marshal(map[string]string{"pointer": pointerLine("b-1", "https://stripe.com/checkout", "element", headingItem)})

	out, err := call(t, w.a, "browser_resolve", string(args))
	require.NoError(t, err)
	require.Contains(t, out, "navigated")
	require.Contains(t, out, "https://x.test")
}

func TestBrowserResolve_BadInputIsAHelpfulError(t *testing.T) {
	w := newBrowserWorld(t)
	w.peer.reportTabs([2]string{"b-1", "chat-a"})
	many := make([]string, 11)
	for i := range many {
		many[i] = headingItem
	}
	for name, tc := range map[string]struct{ pointer, want string }{
		"no data line":  {"just prose", "no '⌖data"},
		"bad json":      {"⌖data {nope", "not valid JSON"},
		"wrong version": {`⌖data {"v":2,"tab":"b-1","items":[{}]}`, "version 2"},
		"no tab":        {`⌖data {"v":1,"items":[{}]}`, "no tab id"},
		"no items":      {`⌖data {"v":1,"tab":"b-1","items":[]}`, "no items"},
		"too many":      {pointerLine("b-1", "u", "multi", many...), "limit is 10"},
		"oversize":      {strings.Repeat("a", 70<<10), "limit is"},
	} {
		args, _ := json.Marshal(map[string]string{"pointer": tc.pointer})
		_, err := call(t, w.a, "browser_resolve", string(args))
		require.ErrorContains(t, err, tc.want, name)
	}
	_, err := call(t, w.a, "browser_resolve", `{"pointer":"x","detail":"bogus"}`)
	require.ErrorContains(t, err, "detail must be")
	_, err = call(t, w.a, "browser_resolve", `{"pointer":"x","chatId":"chat-b"}`)
	require.Error(t, err)
	require.Empty(t, w.peer.ran())
}

func TestBrowserResolve_ForeignAndUnknownTabsAreIndistinguishable(t *testing.T) {
	w := newBrowserWorld(t)
	w.peer.reportTabs([2]string{"mine", "chat-a"}, [2]string{"theirs", "chat-b"})
	resolve := func(tab string) string {
		args, _ := json.Marshal(map[string]string{"pointer": pointerLine(tab, "https://x.test", "element", headingItem)})
		out, err := call(t, w.a, "browser_resolve", string(args))
		require.NoError(t, err)
		return out
	}

	foreign, unknown := resolve("theirs"), resolve("guessed")
	require.Equal(t, strings.ReplaceAll(foreign, "theirs", "X"), strings.ReplaceAll(unknown, "guessed", "X"))
	require.Contains(t, foreign, browserbridge.ErrUnknownTab.Error())
	require.Contains(t, foreign, `h2`, "the pointer's own gist is still echoed")
	require.Empty(t, w.peer.ran(), "nothing reaches the desktop")

	w.peer.reportTabs([2]string{"theirs", "chat-b"}) // "mine" closed
	require.Contains(t, resolve("mine"), browserbridge.ErrUnknownTab.Error())
}

func TestBrowserResolve_ADetachedDesktopLeavesTheTabUnknown(t *testing.T) {
	w := newBrowserWorld(t)
	w.peer.reportTabs([2]string{"b-1", "chat-a"})
	w.peer.session.Close()
	args, _ := json.Marshal(map[string]string{"pointer": pointerLine("b-1", "u", "element", headingItem)})
	text, err := call(t, w.a, "browser_resolve", string(args))
	require.NoError(t, err, "a closed session forgets its tabs, so the tab is simply unknown")
	require.Contains(t, text, browserbridge.ErrUnknownTab.Error())
}
