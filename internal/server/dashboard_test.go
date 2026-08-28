package server_test

import (
	"encoding/json"
	"io"
	"net/http"
	"os/exec"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDashboardIsServedWithoutAToken(t *testing.T) {
	ts, _, _, _ := newServer(t)

	resp, err := ts.Client().Get(ts.URL + "/")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Contains(t, resp.Header.Get("Content-Type"), "text/html")

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Contains(t, string(body), "resource controller")
	// The page must not embed a token.
	require.NotContains(t, strings.ToLower(string(body)), "bearer ct")
}

func TestDashboardServesVendoredAnime(t *testing.T) {
	ts, _, _, _ := newServer(t)

	resp, err := ts.Client().Get(ts.URL + "/dashboard/anime.umd.min.js")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, "text/javascript; charset=utf-8", resp.Header.Get("Content-Type"))
	require.Equal(t, "public, max-age=31536000, immutable", resp.Header.Get("Cache-Control"))

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Contains(t, string(body), "Anime.js - UMD minified bundle")
	require.Contains(t, string(body), "t.animate=")
	require.Contains(t, string(body), "t.stagger=")
}

func TestDashboardRejectsUnknownAssetPaths(t *testing.T) {
	tests := []struct {
		name string
		path string
	}{
		{name: "source map", path: "/dashboard/anime.umd.min.js.map"},
		{name: "trailing slash", path: "/dashboard/anime.umd.min.js/"},
		{name: "extra suffix", path: "/dashboard/anime.umd.min.jsx"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts, _, _, _ := newServer(t)

			resp, err := ts.Client().Get(ts.URL + tt.path)
			require.NoError(t, err)
			defer resp.Body.Close()
			require.Equal(t, http.StatusNotFound, resp.StatusCode)
		})
	}
}

// dashboardBody fetches the page exactly as a browser would, so every
// assertion below is made against the bytes that actually ship rather than
// against the file on disk.
func dashboardBody(t *testing.T) string {
	t.Helper()
	ts, _, _, _ := newServer(t)

	resp, err := ts.Client().Get(ts.URL + "/")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return string(body)
}

// TestDashboardNeverTouchesLocalStorage pins the storage half of the page's
// credential story: sessionStorage dies with the tab, localStorage does not.
// A client token written to localStorage would outlive the browsing session
// on a shared operator workstation, so the page must not name that API at
// all — not for the token, not for a UI preference, not anywhere.
func TestDashboardNeverTouchesLocalStorage(t *testing.T) {
	require.NotContains(t, dashboardBody(t), "localStorage")
}

var sessionSetItem = regexp.MustCompile(`sessionStorage\.setItem\(`)
var sessionSetItemLiteralKey = regexp.MustCompile(`sessionStorage\.setItem\("([^"]*)"`)

// TestDashboardStoresOnlyTheClientTokenInSessionStorage is the other half:
// the ONE thing the page is allowed to keep for the tab is the client token,
// which reaches only the operator's own jobs. The admin token used to clear a
// device must never be written anywhere, so any second key here — or a
// computed key whose value cannot be read off the source — is a failure.
func TestDashboardStoresOnlyTheClientTokenInSessionStorage(t *testing.T) {
	body := dashboardBody(t)

	writes := sessionSetItem.FindAllString(body, -1)
	require.NotEmpty(t, writes, "the page should still store the client token for the tab")

	keyed := sessionSetItemLiteralKey.FindAllStringSubmatch(body, -1)
	require.Len(t, keyed, len(writes),
		"every sessionStorage.setItem must use a literal key, so this test can see what is stored")
	for _, m := range keyed {
		require.Equal(t, "rc_token", m[1], "unexpected sessionStorage key written by the dashboard")
	}
}

// TestDashboardPromptsForTheAdminTokenAndKeepsNothing pins the admin path:
// the token is asked for at the moment of the click, handed to exactly one
// request, and dropped before any promise callback could capture it. The
// nulling line is asserted directly because it is load-bearing, not
// decorative — without it the value stays reachable from the closure for as
// long as the page lives.
func TestDashboardPromptsForTheAdminTokenAndKeepsNothing(t *testing.T) {
	body := dashboardBody(t)

	require.Contains(t, body, "window.prompt(", "clearing a device must prompt for an admin token")
	require.Contains(t, body, `"/v1/devices/" + encodeURIComponent(id) + "/clear"`)
	require.Contains(t, body, `Authorization: "Bearer " + admin`,
		"the clear request must use the just-prompted value")
	// Anchored to the start of a line rather than asserted as a plain
	// substring: `require.Contains(body, "admin = null;")` also passes when
	// the drop has been commented out, which is exactly the regression this
	// is here to catch. Verified by commenting the line out and watching
	// this trip.
	require.Regexp(t, `(?m)^\s*admin = null;`, body,
		"the prompted admin token must be dropped before any callback can capture it")
	// There is a prompt per admin action (clear, retire) rather than exactly
	// one, so counting them would only track how many admin actions exist.
	// What must stay true is that EVERY prompt on this page asks for an admin
	// token: a prompt for anything else — a name, a reason, a command — would
	// be a new way to put user input into a request, which is the thing this
	// assertion is really guarding.
	prompts := promptCall.FindAllStringSubmatch(body, -1)
	require.NotEmpty(t, prompts, "clearing a device must prompt for an admin token")
	for _, m := range prompts {
		require.Contains(t, strings.ToLower(m[1]), "admin token",
			"every prompt on this page must be an admin-token prompt; found: %s", m[1])
	}
}

// promptCall captures the message a window.prompt() call shows.
var promptCall = regexp.MustCompile(`window\.prompt\(\s*"([^"]*)"`)

// assignment matches `target = value` (not ==, ===, !=, <=, >= or =>) and
// captures the assignment target and everything up to the statement's
// semicolon.
var assignment = regexp.MustCompile(`(?s)([A-Za-z_$][\w$.]*)\s*=[^=>]([^;]*)`)

// quoted matches a string literal, stripped from an assignment's right-hand
// side before looking for the admin token there: `var note = "admin token
// used";` copies nothing, and a test that cannot tell it from `stash =
// admin;` would be noise rather than a guard.
var quoted = regexp.MustCompile(`"[^"]*"|'[^']*'`)

var adminIdentifier = regexp.MustCompile(`\badmin\b`)

// TestDashboardNeverCopiesTheAdminTokenAnywhere closes the one regression
// the assertions above cannot see. Keeping `admin = null;` exactly where it
// is, while adding a module-level `var lastAdmin` and `lastAdmin = admin;`
// just before the drop, passes every other test on this page and retains
// the admin token for the life of the tab — and it is exactly what someone
// adding a "retry the clear" affordance would write.
//
// So: between the prompt and the drop, `admin` may be assigned TO (it is
// trimmed there), but its value may never be assigned to anything else.
// Only that window is examined, because that is the only place in the file
// where the identifier holds a real token at all.
func TestDashboardNeverCopiesTheAdminTokenAnywhere(t *testing.T) {
	body := dashboardBody(t)

	start := strings.Index(body, "window.prompt(")
	require.NotEqual(t, -1, start, "the admin prompt must exist")
	drop := regexp.MustCompile(`(?m)^\s*admin = null;`).FindStringIndex(body[start:])
	require.NotNil(t, drop, "the drop must exist and must follow the prompt")
	span := body[start : start+drop[1]]

	for _, m := range assignment.FindAllStringSubmatch(span, -1) {
		target, value := m[1], quoted.ReplaceAllString(m[2], "")
		if target == "admin" {
			continue // assigning to admin itself is the trim, and the drop
		}
		// The request itself is the one destination the token is allowed to
		// reach: `var pending = fetch(... "Bearer " + admin ...)` binds a
		// promise, not the token. Every other assignment binds whatever
		// `admin` evaluates to, which is the copy this test is about.
		if strings.HasPrefix(strings.TrimSpace(value), "fetch(") {
			continue
		}
		require.False(t, adminIdentifier.MatchString(value),
			"the prompted admin token is copied out of `admin` by `%s =%s`: "+
				"between the prompt and the drop its value may reach the request and nothing else",
			target, m[2])
	}
}

// TestDashboardShipsNoTokenLiteral guards the served asset against a
// credential ever being baked into it — the admin token above all, since it
// clears devices fleet-wide, but the worker and client tokens too. The page
// is served to anyone who can reach the port, with no token at all.
func TestDashboardShipsNoTokenLiteral(t *testing.T) {
	body := dashboardBody(t)

	// The exact tokens newServer configures. If the page ever embedded the
	// admin token it would be this string.
	for _, tok := range []string{"atok", "ctok", "wtok"} {
		require.NotContains(t, body, tok)
	}
}

// TestDashboardComputesLabelAgeServerSideNotViaDateParse pins the fix for
// the one age on this page that used to be computed in the browser: an
// operator's own clock, not the controller's, used to decide whether a
// device's labels looked stale, via Date.parse(label.updated_at) measured
// against Date.now(). The page must now read oldest_label_age_seconds
// straight off each DeviceView instead.
//
// Asserted as the call syntax "Date.parse(" specifically, not the bare
// substring "Date.parse" — the surrounding code's own comment explaining
// this history necessarily still mentions the name, and a substring-only
// assertion would fail on that prose rather than on an actual call.
func TestDashboardComputesLabelAgeServerSideNotViaDateParse(t *testing.T) {
	body := dashboardBody(t)
	require.NotContains(t, body, "Date.parse(",
		"label staleness must come from the controller's oldest_label_age_seconds, not a browser-side Date.parse call on updated_at")
}

// markupSinks are the DOM APIs that turn a string into markup. The page
// renders values nobody here controls — a job's command line is written by
// whoever submitted it, a usage sheet by whoever administers the host, an
// error string by the controller — and the only reason none of them can
// become an element is that every one of them is inserted as a text node.
// The usage-sheet renderer in particular is a Markdown renderer, which is
// exactly the kind of code someone "simplifies" back into a string of HTML.
var markupSinks = []string{
	".innerHTML", ".outerHTML", "insertAdjacentHTML(", "document.write(",
	"createContextualFragment(",
}

// TestDashboardNeverAssignsMarkup pins that boundary on the shipped asset.
func TestDashboardNeverAssignsMarkup(t *testing.T) {
	body := dashboardBody(t)
	for _, sink := range markupSinks {
		require.NotContains(t, body, sink,
			"every user-controlled string on this page must be inserted as a text node")
	}
}

// dashboardFunction returns a complete function declaration from the
// JavaScript that is actually served. The dashboard keeps these helpers
// deliberately small and free of template literals, so matching braces is
// enough to isolate them for execution by Node.
func dashboardFunction(t *testing.T, body, name string) string {
	t.Helper()
	start := strings.Index(body, "function "+name+"(")
	require.NotEqual(t, -1, start, "dashboard must ship function %s", name)
	open := strings.Index(body[start:], "{")
	require.NotEqual(t, -1, open)
	open += start
	depth := 0
	for i := open; i < len(body); i++ {
		switch body[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return body[start : i+1]
			}
		}
	}
	t.Fatalf("function %s has no closing brace", name)
	return ""
}

func runDashboardJS(t *testing.T, functions []string, expression string, out any) {
	t.Helper()
	body := dashboardBody(t)
	var source strings.Builder
	for _, name := range functions {
		source.WriteString(dashboardFunction(t, body, name))
		source.WriteByte('\n')
	}
	source.WriteString("process.stdout.write(JSON.stringify(")
	source.WriteString(expression)
	source.WriteString("));")
	output, err := exec.Command("node", "-e", source.String()).CombinedOutput()
	require.NoError(t, err, "dashboard JavaScript failed: %s", output)
	require.NoError(t, json.Unmarshal(output, out), "dashboard JavaScript returned %q", output)
}

func runDashboardJSWithPrelude(t *testing.T, functions []string, prelude, expression string, out any) {
	t.Helper()
	body := dashboardBody(t)
	var source strings.Builder
	source.WriteString(prelude)
	source.WriteByte('\n')
	for _, name := range functions {
		source.WriteString(dashboardFunction(t, body, name))
		source.WriteByte('\n')
	}
	source.WriteString("Promise.resolve(")
	source.WriteString(expression)
	source.WriteString(").then(function (value) { process.stdout.write(JSON.stringify(value)); })")
	output, err := exec.Command("node", "-e", source.String()).CombinedOutput()
	require.NoError(t, err, "dashboard JavaScript failed: %s", output)
	require.NoError(t, json.Unmarshal(output, out), "dashboard JavaScript returned %q", output)
}

func TestDashboardFleetOverview(t *testing.T) {
	body := dashboardBody(t)
	for _, id := range []string{
		`id="fleetOverview"`, `id="fleetSummary"`, `id="fleetFacts"`,
		`id="machineRoom"`, `id="attentionRegion"`, `id="workStrip"`, `id="fleetMode"`,
	} {
		require.Contains(t, body, id)
	}
	for _, fn := range []string{"renderOverview", "renderMachineRoom", "renderAttention", "renderWorkStrip"} {
		require.Contains(t, body, "function "+fn+"(")
	}
	require.Contains(t, body, `<script src="/dashboard/anime.umd.min.js"></script>`)
	require.NotContains(t, body, `class="board"`)
	require.NotContains(t, strings.ToLower(body), "topology")
}

func TestDashboardMachineRoom(t *testing.T) {
	body := dashboardBody(t)
	require.Contains(t, body, "function hostSeverity(views)")
	require.Contains(t, body, "hostSeverity(byHost[b]) - hostSeverity(byHost[a])")
	require.Contains(t, body, `a.device.id.localeCompare(b.device.id)`)
	require.Contains(t, body, `stateWord(d.state)`)
	require.Contains(t, body, `v.quarantine_reason`)
	require.Contains(t, body, `waitingBy[v.device.id]`)
}

func TestDashboardMatrixMode(t *testing.T) {
	body := dashboardBody(t)
	require.Contains(t, body, "function setFleetMode(mode)")
	require.Contains(t, body, `mode !== "matrix" ? "bays" : "matrix"`)
	require.Contains(t, body, `room.classList.toggle("matrix", fleetMode === "matrix")`)
	require.Contains(t, body, `views.slice()`)
	require.Contains(t, body, `el("table", "matrix-table")`)
	require.Contains(t, body, `data-device-id`)
}

func TestDashboardFleetFreshness(t *testing.T) {
	tests := []struct {
		name  string
		views string
		want  map[string]any
	}{
		{name: "all missing", views: `[{}, {"oldest_label_age_seconds":null}]`, want: map[string]any{"missing": float64(2), "future": float64(0), "oldest": nil}},
		{name: "mixed missing and known", views: `[{"oldest_label_age_seconds":41}, {}]`, want: map[string]any{"missing": float64(1), "future": float64(0), "oldest": float64(41)}},
		{name: "future only", views: `[{"oldest_label_age_seconds":-1}, {"oldest_label_age_seconds":-9}]`, want: map[string]any{"missing": float64(0), "future": float64(2), "oldest": nil}},
		{name: "mixed missing future and known", views: `[{"oldest_label_age_seconds":73}, {}, {"oldest_label_age_seconds":-2}]`, want: map[string]any{"missing": float64(1), "future": float64(1), "oldest": float64(73)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got map[string]any
			runDashboardJS(t, []string{"fleetFreshness"}, "fleetFreshness("+tt.views+")", &got)
			require.Equal(t, tt.want, got)
		})
	}
	body := dashboardBody(t)
	require.Contains(t, body, `if (fresh.missing) {`)
	require.Contains(t, body, `if (fresh.future) {`)
}

func TestDashboardWorkEligibilityMatchesControllerSelectorSemantics(t *testing.T) {
	expression := `(() => {
deviceIndex = {
  large:{device:{labels:[{key:"vram",value:"80G",source:"detected"}]}},
  small:{device:{labels:[{key:"vram",value:"24G",source:"detected"}]}},
  unknown:{device:{labels:[{key:"model",value:"h100",source:"detected"}]}}
};
return [
  eligibleDeviceIDs({selector:"vram>=40G"}),
  eligibleDeviceIDs({selector:"vram<=40G"}),
  eligibleDeviceIDs({selector:"vendor!=amd"})
];
})()`
	var got [][]string
	runDashboardJS(t, []string{"labelMap", "selectorQuantity", "selectorEligible", "eligibleDeviceIDs"}, expression, &got)
	require.Equal(t, [][]string{{"large"}, {"small"}, {}}, got)
}

func TestDashboardFullWorkspaces(t *testing.T) {
	body := dashboardBody(t)
	for _, id := range []string{`id="workspace"`, `id="workspaceHeading"`, `id="workspaceBody"`, `id="workspaceBack"`} {
		require.Contains(t, body, id)
	}
	for _, fn := range []string{"renderHostWorkspace", "renderDeviceWorkspace", "renderJobWorkspace"} {
		require.Contains(t, body, "function "+fn+"(")
	}
	for _, removed := range []string{`id="scrim"`, `id="panel"`, `role="dialog"`, `aria-modal="true"`, "renderDevicePanel", "renderJobPanel"} {
		require.NotContains(t, body, removed)
	}
	var related []string
	runDashboardJS(t, []string{"relatedDeviceViews"}, `(() => {
deviceIndex={a:{device:{id:"a",host:"rack"}},b:{device:{id:"b",host:"rack"}},c:{device:{id:"c",host:"other"}}};
return relatedDeviceViews(deviceIndex.a).map(function(v){return v.device.id});
})()`, &related)
	require.Equal(t, []string{"b"}, related)
}

func TestDashboardWorkspaceRouting(t *testing.T) {
	prelude := `
function node() { return {classList:{values:{},add:function(k){this.values[k]=true},remove:function(k){delete this.values[k]}},style:{},focus:function(){document.activeElement=this}} }
var nodes={fleetOverview:node(),whoBar:node(),workspace:node(),workspaceKind:node()};
var document={activeElement:null,contains:function(n){return !!n},querySelectorAll:function(){return []},getElementById:function(id){return nodes[id]}};
var trigger=node(); document.activeElement=trigger;
var window={scrollY:73,location:{hash:"#fleet"},history:{replaceState:function(_a,_b,h){window.location.hash=h}},scrollTo:function(_x,y){window.scrollY=y},matchMedia:function(){return {matches:true}}};
var workspaceView={kind:"fleet"}, workspaceBody=node(), workspaceHeading=node(), workspaceTrigger=null, workspaceTriggerView=null;
workspaceBody.appendChild=function(){}; workspaceBody.textContent="";
var overviewScrollY=0,overviewMode="bays",fleetMode="matrix",reduceMotion={matches:true},rendered="";
function dropLogBlock(){} function setFleetMode(m){fleetMode=m} function renderHostWorkspace(id){rendered="host:"+id}
function renderDeviceWorkspace(id){rendered="device:"+id} function renderJobWorkspace(id){rendered="job:"+id}
`
	var got map[string]any
	runDashboardJSWithPrelude(t,
		[]string{"routeHash", "navigateTo", "routeFromHash", "openDevice", "closeWorkspace", "resetJobWorkspaceTab", "transitionWorkspace", "findWorkspaceTrigger", "renderWorkspace"},
		prelude, `(() => {
  openDevice("gpu / 1"); routeFromHash();
  var opened={hash:window.location.hash,rendered:rendered,headingFocused:document.activeElement===workspaceHeading,overviewHidden:!!nodes.fleetOverview.classList.values.hide};
  closeWorkspace();
  var closed={hash:window.location.hash,triggerFocused:document.activeElement===trigger,scrollY:window.scrollY,mode:fleetMode};
  window.location.hash="#host/rack%2Fa"; routeFromHash();
  var browserRoute={kind:workspaceView.kind,id:workspaceView.id,rendered:rendered};
  window.location.hash="#fleet"; routeFromHash();
  var browserBack={kind:workspaceView.kind,triggerFocused:document.activeElement===trigger,scrollY:window.scrollY,mode:fleetMode};
  return {opened:opened,closed:closed,browserRoute:browserRoute,browserBack:browserBack};
})()`, &got)
	require.Equal(t, map[string]any{
		"opened":       map[string]any{"hash": "#device/gpu%20%2F%201", "rendered": "device:gpu / 1", "headingFocused": true, "overviewHidden": true},
		"closed":       map[string]any{"hash": "#fleet", "triggerFocused": true, "scrollY": float64(73), "mode": "matrix"},
		"browserRoute": map[string]any{"kind": "host", "id": "rack/a", "rendered": "host:rack/a"},
		"browserBack":  map[string]any{"kind": "fleet", "triggerFocused": true, "scrollY": float64(73), "mode": "matrix"},
	}, got)
}

func TestDashboardWorkspaceAccessibility(t *testing.T) {
	body := dashboardBody(t)
	require.Contains(t, body, `<h2 id="workspaceHeading" tabindex="-1"></h2>`)
	var got string
	runDashboardJSWithPrelude(t, []string{"handleWorkspaceKeydown"},
		`var workspaceView={kind:"device"}; var closed=0; function closeWorkspace(){closed++}`,
		`(() => { handleWorkspaceKeydown({key:"Escape"}); handleWorkspaceKeydown({key:"Enter"}); return String(closed); })()`, &got)
	require.Equal(t, "1", got)
}

func TestDashboardWorkspaceJobTabPersistsAcrossRepaints(t *testing.T) {
	var got []string
	runDashboardJSWithPrelude(t, []string{"jobWorkspaceTabFor", "selectJobWorkspaceTab", "resetJobWorkspaceTab", "buildJobWorkspaceTabs"},
		`var jobWorkspaceTab="details", jobWorkspaceTabID=null;
function n(){return {attrs:{},children:[],listeners:{},setAttribute:function(k,v){this.attrs[k]=v},appendChild:function(v){this.children.push(v)}}}
function el(){return n()} function button(_c,_t,fn){var b=n();b.listeners.click=fn;return b}`,
		`(() => { var out=[]; var first=n(); var tabs=buildJobWorkspaceTabs("one",first); out.push(first.attrs["data-tab"]); tabs.children[1].listeners.click(); var repaint=n(); buildJobWorkspaceTabs("one",repaint); out.push(repaint.attrs["data-tab"]); var next=n(); buildJobWorkspaceTabs("two",next); out.push(next.attrs["data-tab"]); resetJobWorkspaceTab(); var closed=n(); buildJobWorkspaceTabs("two",closed); out.push(closed.attrs["data-tab"]); return out; })()`, &got)
	require.Equal(t, []string{"details", "output", "details", "details"}, got)
}

func TestDashboardHostWorkspaceLoadsEveryDevicesHistory(t *testing.T) {
	prelude := `
var workspaceView={kind:"host",id:"rack"}, describes={}, jobIndex={};
var calls=[];
function authHeaders(){return {}}
var fetch=function(url){calls.push(url); var id=decodeURIComponent(url.split("/")[3]); return Promise.resolve({ok:true,json:function(){return Promise.resolve({recent_jobs:[{id:"shared",device_id:id},{id:"done-"+id,device_id:id}]})}})};
`
	var got map[string]any
	runDashboardJSWithPrelude(t, []string{"fetchHostWorkspaceData"}, prelude,
		`fetchHostWorkspaceData("rack",[{device:{id:"a"}},{device:{id:"b"}}]).then(function(jobs){return {calls:calls,jobs:jobs.map(function(j){return j.id}),indexed:Object.keys(jobIndex).sort()}})`, &got)
	require.Equal(t, []any{"/v1/devices/a/describe", "/v1/devices/b/describe"}, got["calls"])
	require.Equal(t, []any{"shared", "done-a", "done-b"}, got["jobs"])
	require.Equal(t, []any{"done-a", "done-b", "shared"}, got["indexed"])
}

func TestDashboardPreservesAdminActions(t *testing.T) {
	body := dashboardBody(t)
	for _, preserved := range []string{"clearDevice(d.id, v.quarantine_reason)", "retireDevice(d.id)", "killJob({ id: id, device_id: job.device_id, submitter: job.submitter })", "copyText(full)", "renderMarkdown(data.sheet)"} {
		require.Contains(t, body, preserved)
	}
}

func TestDashboardReducedMotion(t *testing.T) {
	var got map[string]any
	runDashboardJSWithPrelude(t, []string{"transitionWorkspace"},
		`var calls=0; var window={anime:true}; var anime={animate:function(){calls++}}; var reduceMotion={matches:true};`,
		`(() => { var reduced={style:{}}; transitionWorkspace(reduced); reduceMotion.matches=false; var moving={style:{}}; transitionWorkspace(moving); return {reduced:reduced.style.opacity,moving:moving.style.opacity,calls:calls}; })()`, &got)
	require.Equal(t, map[string]any{"reduced": "1", "moving": "0", "calls": float64(1)}, got)
}

// glyphTemplate matches one of the device glyphs declared as static markup.
var glyphTemplate = regexp.MustCompile(`(?s)<template id="glyph-([a-z]+)">\s*(?:<!--.*?-->\s*)?<svg`)

// TestDashboardDrawsItsGlyphsFromStaticTemplates pins how the device glyphs
// reach the page. They are the one part of it that is a picture rather than
// a sentence, and a picture is exactly what somebody reaches for a string of
// markup to build — `svg.innerHTML = "<rect …>"` is the obvious way to write
// this and would take the whole no-markup boundary with it (see
// TestDashboardNeverAssignsMarkup, which would also catch that, and this,
// which says what to do INSTEAD).
//
// So: every glyph is a <template> in the served asset, put on the page by
// cloneNode. Nothing is assembled at runtime, which means no device ID, no
// label a host wrote, and no state name can ever be inside markup.
func TestDashboardDrawsItsGlyphsFromStaticTemplates(t *testing.T) {
	body := dashboardBody(t)

	kinds := glyphTemplate.FindAllStringSubmatch(body, -1)
	require.NotEmpty(t, kinds, "the device glyphs must be inline <template> SVG in the served asset")
	for _, m := range kinds {
		require.Contains(t, body, `document.getElementById("glyph-" + glyphKind(v))`,
			"a glyph must be looked up by kind, not built")
		require.NotEmpty(t, m[1])
	}
	require.Contains(t, body, ".content.firstElementChild.cloneNode(true)",
		"a glyph reaches the page by cloning its template, never as a string of markup")
}

// TestDashboardChoosesGlyphsFromLabelsWithAFallback pins the rule that keeps
// this page honest about a fleet it has never seen. A glyph is picked from
// the labels the controller was given — never from a list of host names, so
// the fourth box renders on the day its worker registers rather than on the
// day somebody edits this file — and anything that matches no rule falls
// back to the generic die. Delete the fallback and an unrecognised device
// renders as nothing at all, which is the failure this guards.
func TestDashboardChoosesGlyphsFromLabelsWithAFallback(t *testing.T) {
	body := dashboardBody(t)

	require.Contains(t, body, "function glyphKind(v)")
	require.Contains(t, body, "var m = labelMap(v), text = \"\";",
		"the glyph must be decided from this device's labels")
	require.Regexp(t, `(?m)^\s*return "die";`, body,
		"a device whose labels match no rule must still get a glyph")
	// The one shape that is NOT chosen from the hardware: a device out of
	// the pool reads as wrong by shape, whatever it is made of.
	require.Contains(t, body, `if (v.device.state === "unhealthy") return "fault";`)
}

// TestDashboardCarriesStateInWordsNotOnlyColour pins the accessibility half
// of the board's design. The rail is a colour and the glyph is a shape, but
// the thing that has to survive colourblindness, a bad monitor and a glance
// from the side is the WORD — so the state pill carries stateWord()'s text,
// and the glyphs, which repeat what the words already say, are hidden from a
// screen reader rather than read out twice.
func TestDashboardCarriesStateInWordsNotOnlyColour(t *testing.T) {
	body := dashboardBody(t)

	require.Contains(t, body, `el("span", "pill " + d.state, stateWord(d.state))`,
		"the state pill must say the state in words")
	require.Equal(t, strings.Count(body, "<svg viewBox=\"0 0 48 48\" class=\"glyph\""),
		strings.Count(body, "<svg viewBox=\"0 0 48 48\" class=\"glyph\" aria-hidden=\"true\""),
		"every device glyph is decorative and must be hidden from a screen reader")
}

// externalRef matches a src= or href= pointing at another host: an absolute
// http(s) URL or a protocol-relative one.
var externalRef = regexp.MustCompile(`(?i)(src|href)\s*=\s*["']?\s*(https?:)?//`)

// TestDashboardMakesNoExternalRequests pins the security boundary the page
// exists behind: it is served to anyone who can reach the controller's port,
// on networks where the controller itself may be the only thing reachable,
// and it must fetch nothing from anywhere. No CDN, no font, no image, no
// analytics — everything inline. The favicon is a data: URI for this reason
// and passes, since it names no host.
func TestDashboardMakesNoExternalRequests(t *testing.T) {
	body := dashboardBody(t)
	require.NotRegexp(t, externalRef, body,
		"the dashboard must reference no external asset of any kind")
}

func TestUnknownPathIs404NotTheDashboard(t *testing.T) {
	ts, _, _, _ := newServer(t)

	resp, err := ts.Client().Get(ts.URL + "/nope")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
}

// A non-GET request to an unknown path is 405, not 404: "GET /" is
// registered as a subtree pattern (it ends in "/"), so net/http's ServeMux
// matches the path for any method and then reports 405 itself once it sees
// no method matches — handleDashboard's own r.URL.Path != "/" check never
// even runs. That is standard library behaviour, not a bug, and it is
// defensible (a 405 correctly tells the caller the path exists under a
// different method rather than not at all) — this test exists so that
// behaviour is a pinned decision rather than an accident nobody would
// notice changing.
func TestUnknownPathNonGetIs405(t *testing.T) {
	ts, _, _, _ := newServer(t)

	req, err := http.NewRequest(http.MethodPost, ts.URL+"/nope", nil)
	require.NoError(t, err)
	resp, err := ts.Client().Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode)
}
