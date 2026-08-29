# Giving the fleet dashboard a clock

## Goal

Change what the dashboard is *about*. Today it renders a snapshot: the state of
every device at this instant, refreshed every five seconds. Almost none of the
questions an operator opens it to answer have that shape.

- When does a card free up?
- Has this queue wait gone abnormal, or is the fleet simply full?
- Did this card just break, or has it been flapping since lunch?
- What happened while I was away?

Each of those is a question about time, and a list of rows has no time in it.
This redesign makes the working day the primary object: devices become lanes,
jobs become bars across those lanes, now is a line, and the queue sits to the
right of that line as the future it actually is.

The known-problems section of `.impeccable.md` has said this from the start —
"no sense of time or trend, everything is an instantaneous snapshot" — and two
previous passes at the dashboard rearranged the rows instead.

## What this is not

This does not restart the workspace redesign that merged in #2. Full-width
entity workspaces, browser-history routing, focus restoration, reduced-motion
handling and the attention region are all kept and become the detail layer
underneath the timeline. The job, device and host workspaces gain a time axis;
they are not rebuilt.

## The data already exists

The controller has served the working day since the job-history work landed.
The dashboard has never asked for it.

- `GET /v1/jobs?limit=200` returns `submitted_at`, `started_at`, `finished_at`,
  `device_id`, `submitter`, `state` and `exit_code` per job. That is every bar
  on the chart.
- `max_runtime_seconds` and `idle_timeout_seconds` are on the job. A running
  job's upper bound is therefore a fact, not an estimate, and the future to the
  right of the now-line is bounded rather than guessed.
- `queued_waiting_seconds` on the state response gives queue waits already
  measured against the controller's clock.
- Host and device utilization for any window is derived from the same job
  records. It needs no endpoint and no stored metric.

One thing is stored but not reachable. `device_recoveries` records every time a
device returned to the pool on its own, and `AutoRecover` counts against it —
at most `autoRecoveryLimit` (3) inside `autoRecoveryWindow` (1 hour), after
which the device waits for a person. This is precisely the difference between
"this card is out" and "this card has used up its automatic returns", and the
dashboard cannot see it. Exposing it is the only backend change in scope.

## The controller owns the clock

`client_api.go` computes every age and wait server-side on purpose, and says
why: a reader's clock must not be able to make a stuck queue look fresh. A
timeline drawn from absolute timestamps against `Date.now()` would undo that
guarantee for the whole page rather than one field.

So the axis is anchored on the controller. Every response already carries a
`Date` header, which is the controller's own clock and costs no API change. The
client records the offset between that header and the browser clock on each
refresh and derives every position, tick and duration from the corrected value.
A browser an hour behind draws the same picture as one that is correct.

`Date.now()` may still drive animation timing and nothing else.

## The Day

### Reading the chart

Occupancy is ink; availability is whitespace. Free capacity is the gap between
bars, not a green fill — eight lanes painted green is both louder and slower to
read than eight lanes with holes in them, and it disagrees with a brief that
asks the page to stay quiet until something is wrong.

Bars carry one categorical distinction only: work you submitted against
everyone else's. That pair was validated for colour-vision deficiency at
ΔE 21.4 normal and 15.7 protan, and the submitter is written on any bar wide
enough to hold it, so identity never rests on hue. Everything else on the chart
is status — failed runs, quarantine, silence — drawn from the reserved status
palette and always accompanied by a word or a texture.

### Texture is a fill, never a background for type

Quarantine and not-reporting spans are drawn as a 45-degree hatch, which is
what keeps them legible in greyscale and for a colourblind reader. Type is
never set on that hatch. Where such a span needs a label, the label sits on an
opaque plate at the head of the span and the texture begins after it.

The plate says *when the state began and how long it has held*. It does not
repeat the state word, which the lane label already carries and the legend
already defines. A span too narrow for its plate clips the plate rather than
smearing text across stripes.

This rule applies to every textured span the page ever grows, not only these
two.

### Layout

Lanes are grouped under their host. A host header carries the host name, its
hardware summary and heartbeat freshness, and is itself a link to the host
workspace. Within a host, lanes sort by device ID; hosts sort by operational
severity, then name.

The now-line runs the full height of the chart. Everything right of it is
shaded as future. A queued job that names a device by ID appears in that
device's lane as a dashed outline starting at now — an outline, because it has
not happened. A queued job with a selector rather than a named device belongs
in the queue list below the chart, not projected onto a lane the scheduler has
not chosen.

The default window is the working day, with 6h/12h/day options. Beyond a
threshold where lanes stop being legible, the chart collapses to one row per
host with per-host occupancy, and individual lanes open from the host
workspace. Devices are never hidden to preserve a composition.

### Attention

The attention region from #2 is kept and now states duration and history
rather than only current state: not "gpu3 is unhealthy" but "came back on its
own three times, then failed for good — it has used up its automatic returns".
Each item routes to the entity it concerns. The region is absent when nothing
needs attention.

## Running now

A destination alongside the Day, listing every active lease at once. Watching
three jobs currently means opening three pages one at a time.

Each run shows submitter, card, elapsed against `max_runtime_seconds` as a
bounded meter, the time remaining before the limit stops it, and a short live
tail of its output. A hold shows no tail and says why: a hold runs no command.

The tail is the same log controller as the job workspace at a smaller height.
Tails are bounded to a few lines, and every stream is aborted when the view
closes. This is the screen most able to leak readers on the controller, and the
one that most needs its cleanup pinned by a test.

## Workspaces

### Job

The workspace leads with the life of the run: a strip spanning submitted to the
run's limit, segmented into the queue wait and the run so far, with the moment
the limit will stop it marked. Below it, the output takes the entire remaining
height.

Details, command and actions move to a fixed-width rail beside the output, not
above it. Nobody opens a job to read its working directory. On narrow screens
Details and Output remain tabs.

A queued job has no output yet and says so in a sentence that explains what
happens next, rather than rendering an empty pane. A finished job shows its
stored output with a header that distinguishes completion from interruption.

### Device

Its own single lane across the day, its jobs for the day as a list, and
utilization derived from them. A quarantined device leads with the reason, the
time it left the pool, and its recovery history: when it came back on its own,
how many of its automatic returns remain, and therefore whether clearing it is
a decision or a formality.

### Host

The host's lanes together, utilization across them for the window, and the
usage sheet. When every lane on a host stops within the same second and its
jobs are recorded lost, the workspace says so in words: two cards do not fail
together, so this is the worker rather than the hardware.

Heartbeat history is not stored — only the last heartbeat — so outages are
inferred from clustered lost jobs and stated as an inference. The page does not
chart a heartbeat series it does not have.

## Navigation

Two destinations, three workspaces. The Day and Running now are places you can
be; job, device and host are workspaces you route into and return from.

A breadcrumb bar under the header shows the path and every crumb is a link.
The hash identifies the view — `#day`, `#running`, `#job/<id>`,
`#device/<id>`, `#host/<name>` — so Back works and a view can be handed to
someone else. The token never appears in the hash. Escape returns from a
workspace unless a nested confirmation owns it.

## The one backend change

`DescribeResponse` gains this device's recovery history: the timestamps of
recent automatic returns and how many remain inside the window. The table and
its pruning already exist; this is a read.

It is served to the `client` role. It reveals when a device flapped, which any
holder of a client token can already infer from device state over time.

## Security and asset constraints

Unchanged, and none of them relax for a chart.

- One self-contained `index.html` plus the vendored Anime.js, `go:embed`-ed.
  No external request of any kind.
- Every API-controlled string enters the document as a text node. Bars, plates,
  lane labels, tooltips and usage sheets are built with DOM calls; the page
  gains no markup sink.
- Bar geometry is set through individual style properties, never by assembling
  a style string from values the controller supplied.
- `sessionStorage` holds the client token and nothing else. `localStorage` is
  not named or used. Window and density preferences live in memory for the tab.
- The admin token is prompted per action, sent to exactly one request, and
  nulled before any callback can capture it.

## Accessibility

Bars, plates, lane names, host names and attention items are real buttons.
Every state is available as text, not only as hue or texture. The chart has a
table view carrying the same rows for screen readers and for anyone who wants
the numbers. Focus enters a workspace at its heading and returns to the control
that opened it. Under `prefers-reduced-motion` the chart redraws without
transition.

## Testing

Keep every existing dashboard test. Add:

- source assertions that the axis is anchored on the response `Date` header and
  that no age, wait or bar position is derived from `Date.now()`;
- executed-function tests for the geometry: a bar's start and width for a job
  that begins before the window, ends after it, is still running, or is shorter
  than a pixel;
- utilization derived from known job records, including overlapping and
  zero-length leases;
- a queued job with a named device projecting into that lane, and one with a
  selector staying out of the lanes;
- the log controller surviving a state refresh with its scroll position and
  follow state intact — the regression that outlived two redesigns;
- every stream aborted when Running now closes or a job workspace changes
  subject;
- no markup sink, no `localStorage`, one `sessionStorage` key, admin-token
  disposal;
- textured spans carrying no text node on the textured element itself.

Visual states to cover: an empty fleet; a single device; a fleet past the
collapse threshold; a day with no jobs at all; a job longer than the window; a
device quarantined for the whole window; clustered lost jobs; light and dark.

Run the full Go suite after touching the asset — the dashboard tests assert on
the bytes the handler serves.

## Out of scope

Controller-side event retention, a heartbeat history table, log indexing or
server-side log search, stored metrics, utilization time series, a frontend
build step, and any command line in the browser. Nothing here invents data the
controller does not have; where an answer is an inference, the page says so.
