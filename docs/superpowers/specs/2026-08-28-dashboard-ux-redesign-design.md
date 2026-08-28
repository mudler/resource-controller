# Fleet operator dashboard UX redesign

## Goal

Redesign the embedded web dashboard around the needs of an operator who manages
a shared hardware fleet throughout the day. The dashboard must make fleet
health, device availability, active work, queue pressure, and incidents clear
at a glance, then provide enough room to investigate a host, device, or job
without forcing dense operational data into a narrow side panel.

The redesign also makes stored and live logs usable as an operational tool. A
reader must be able to follow output at the live edge, scroll back without
being pulled down by new output, and return to live output deliberately.

The interface should feel like a precise daylight instrument: composed,
substantial, and quiet during normal operation. Its visual impact comes from
the fleet's structure and from smooth changes of focus, not from ambient
particles, animated connection lines, glowing effects, or decorative motion.

## Product principles

The fleet is the main object. Jobs, events, labels, and actions are presented in
relation to the hosts and devices they affect.

Normal operation is visually quiet. Green is not painted across the whole
page; it confirms availability in compact state indicators. Amber identifies
occupied capacity or queue pressure. Red is reserved for a condition that
requires intervention.

Overview and investigation are different modes. The overview optimizes for
comparison and anomaly detection. A workspace optimizes for reading, diagnosis,
and action. Neither mode is compressed to accommodate the other.

Motion represents a real change in application state or operator focus. The
interface has no continuous decorative animation.

Security boundaries in the current dashboard remain requirements, not
implementation details to relax during the redesign.

## Information architecture

The authenticated application has three primary destinations:

- **Fleet** is the default live overview of hosts and devices.
- **Work** lists queued, active, and recent jobs with fleet-aware filters.
- **History** provides the existing terminal-job history in a form optimized
  for investigation rather than live monitoring.

Work and History remain views within the same document and view-state
controller. They are destinations in the interface, not new server routes.

Selecting a host, device, or job opens a full-width workspace in place of the
overview content. It does not open a modal or a right-hand detail pane. The
browser Back command, an in-page Back control, and Escape return to the previous
view. Returning restores the overview's filters, sort, selected mode, and scroll
position.

The URL hash identifies the current destination and selected entity, for
example `#fleet`, `#device/<encoded-id>`, or `#job/<encoded-id>`. This makes
browser history work without introducing server-side routes. The token never
appears in the hash.

## Fleet overview

### Fleet header

The header answers whether the fleet is healthy before the operator reads any
individual row. It states:

- free, occupied, not-reporting, and unhealthy device counts;
- queued-job count and longest wait;
- oldest worker heartbeat or label freshness problem;
- number of incidents requiring intervention.

These values form one readable summary and a compact aligned facts row. They
must not become a grid of interchangeable metric cards.

The application header also shows connection freshness and current role. Search
or command access can find a host, device, submitter, or job ID. Search is a
client-side filter over data the dashboard has already fetched.

### Machine room

Hosts are rendered as substantial bays. A bay has a stable header containing
the host name, the hardware summary shared by its devices, and heartbeat
freshness. Its body contains the host's devices in an aligned row or small
matrix.

Each device exposes these facts without requiring a click:

- short device ID and hardware kind;
- free, occupied, not-reporting, or unhealthy state in both color and words;
- holder or submitter and current job when occupied;
- elapsed time for active work;
- a concise quarantine reason when unhealthy;
- device-specific queued demand when jobs name that device.

The machine room does not draw graph edges between hosts and a controller. Host
grouping, alignment, labels, and state are sufficient to communicate the fleet
structure. No dots travel between objects. The controller's own connection
state belongs in the application header rather than at the center of a diagram.

Hosts sort by operational severity first, then by host name. Within a host,
devices sort by ID. A filtering or sorting change reorganizes bays with a
layout transition, but their geometry remains readable before and after the
transition.

When the fleet exceeds the space in which bays remain legible, the operator can
switch to a compact matrix. The matrix presents the same state and actions in a
sortable table. Topology is not a separate data model and does not hide devices
to preserve a visual composition.

### Attention region

Active incidents are separated from the healthy fleet through placement and
contrast. The attention region lists unhealthy or long-disconnected devices,
long queue waits, and broken live connections. Each incident states what
happened, how long it has persisted, and the next useful action.

The region is absent when nothing needs attention; the layout gives that space
back to the fleet. It is not a permanent activity sidebar filled with low-value
events.

Recent state changes remain available in a compact live-change feed below the
attention items. The feed explicitly states that it begins when the tab opens
because the controller does not retain an event history to replay.

### Work strip

Queued and active work appears below the machine room. A queued job shows its
wait, selector or named device, priority when non-default, and queue position
when available. Selecting a job highlights its assigned device or the devices
known to match it. This relationship is expressed through temporary highlight
and labels, not connector graphics.

## Entity workspaces

### Host and device workspace

Opening a host or device replaces the overview with a full-width workspace.
The workspace header contains the complete ID, current state, freshness, and
the few available actions. The body separates diagnosis from reference data.

The first region answers what requires attention now. For an unhealthy device,
it leads with the quarantine reason, relevant job, time of failure, and evidence
available from the controller. For a ready device, it leads with availability
and queued demand. For a busy device, it leads with current work, submitter, and
elapsed time.

The reference region contains hardware labels with provenance and age, the host
or device usage sheet, recent jobs, and related devices on the same host. Long
usage sheets receive the reading width they need instead of sharing a narrow
rail with actions and logs.

Device actions preserve current authorization behavior. Killing another
submitter's work requires the existing ownership behavior. Clear and retire
remain admin actions and ask for an admin token at action time.

### Job workspace

The job workspace contains status, submitter, assigned device or selector,
queue position, elapsed or total duration, command, working directory, limits,
exit information, and stop reason. It links back to the assigned device without
nesting another workspace.

On wide screens, job facts and output use resizable adjacent regions. On narrow
screens, Details and Output are tabs. The Output tab remains available after a
job reaches a terminal state.

## Live log viewer

The log viewer is a stable component that is created once for the selected job.
State refreshes update job metadata around it but must not replace the viewer,
restart its request, or reset its scroll position.

The viewer starts at the live edge and follows new output by default. Before an
append, it records whether the reader is at the live edge. If the reader was at
the edge, the append returns the viewport to the new edge. If the reader has
scrolled upward, new output does not change the viewport.

Leaving the live edge changes the viewer to a paused-follow state without
pausing the network stream. A persistent **Jump to live** control appears and
shows the number of unseen appended lines. Reaching the bottom manually
or activating that control resumes follow mode. An explicit Follow/Pause
control provides the same state for keyboard and touch users.

The viewer header distinguishes these states:

- waiting for the response;
- streaming and following;
- streaming while follow is paused;
- completed output;
- interrupted stream;
- reconnecting after an operator request.

An interrupted stream does not silently present itself as completed output.
The current API cannot resume from a byte offset, so reconnecting replays the
stored output. The UI states this behavior and requires explicit confirmation
before replacing the current viewer content and restarting at the live edge.

Incoming chunks are decoded continuously but appended to the DOM in buffered
batches on an animation frame or short timer. This prevents a high-frequency
writer from causing layout work for every network chunk.

The viewer bounds retained DOM text. When the cap is exceeded, it removes the
oldest complete text nodes or line batches and inserts a visible notice that
older output was discarded from the browser view. This cap does not delete
stored controller logs.

The viewer offers text search, line wrapping, copy, and a compact/comfortable
density control. It preserves timestamps already present in output but does not
synthesize timestamps the controller did not record. These preferences remain
in memory for the tab. They must not be stored in `localStorage`, because the
current dashboard deliberately forbids that storage API. Output remains plain
text and is never interpreted as markup.

Closing the workspace or selecting a different job aborts the current log
request immediately and releases its reader.

## Visual system

The application uses a neutral daylight canvas for sustained operator use and
deep graphite navigation or terminal surfaces. The current orange mark remains
the identity accent. Neutral colors are tinted toward the graphite/green hue of
the product rather than generic blue-grey.

The page continues to honor the system's light or dark color preference without
adding a theme switch. The daylight palette is the primary composition. The
dark palette preserves the same hierarchy, contrast, and restrained status
color rather than converting the interface into a glowing terminal theme.

Typography uses a readable humanist sans-serif for interface text and a
contrasting serif or display face only for a small number of fleet-level
statements. Monospace is limited to identifiers, commands, selectors, and log
output. Body text remains at a comfortable size; metadata does not depend on
tiny uppercase labels.

Host bays use precise geometry, visible alignment, modest corner radii, and
either a boundary or a small defined shadow, not both as decoration. Healthy
bays are visually quiet. State colors meet WCAG contrast requirements and are
always accompanied by text or shape.

The design does not use glassmorphism, gradient text, neon glow, decorative
sparklines, animated graph edges, particles, or pulsing backgrounds.

## Motion with Anime.js

Anime.js is vendored with the embedded dashboard asset. The running dashboard
must not depend on a public CDN or any third-party request.

Motion is limited to:

- an overview-to-workspace expansion and the corresponding return;
- layout changes after filtering, sorting, or a responsive mode change;
- a queued job becoming assigned and moving into active work;
- insertion, removal, or severity change of an incident;
- a brief highlight when a device changes state.

Animations use decelerating easing and transform, opacity, clip, or supported
layout-transition techniques. They do not animate arbitrary width, height,
padding, or margin on every frame. A state-change emphasis runs once and
settles. Nothing loops continuously.

With `prefers-reduced-motion: reduce`, workspaces crossfade, layout changes are
immediate, and state changes use static emphasis. All information and actions
remain available.

## Client architecture and data flow

The server continues to embed the dashboard as a self-contained asset. This
redesign edits the existing dashboard asset and introduces no frontend build
pipeline. The vendored Anime.js distribution is embedded locally with the
dashboard and the deployed controller remains self-contained.

Client code is divided into explicit responsibilities:

- a state store owns the latest fleet snapshot and indexes for devices and
  jobs;
- a view-state controller owns destination, selection, filters, sorting,
  restored scroll position, and browser-history integration;
- overview renderers own summaries, host bays, attention items, and work;
- workspace renderers own device, host, and job details;
- one log controller owns the request, decoder, buffered appends, retained
  output, follow state, and cleanup;
- an animation adapter owns Anime.js calls and reduced-motion fallbacks.

The existing SSE stream continues triggering state refreshes. Periodic refresh
remains a fallback. A refresh updates the state store, then performs the
smallest practical view update. It never replaces the active log controller.

The existing state, describe, job, history, action, event, and log endpoints
are the data sources. Backend API changes are out of scope for this redesign.

## Authentication and content safety

The client token remains the only value written to `sessionStorage`. The page
must not name or use `localStorage`.

Admin actions prompt for an admin token at the moment of the action. The token
is sent to exactly one request, is never copied, and is nulled before any
promise callback can capture it. Existing tests that pin this behavior remain
valid.

Device IDs, commands, labels, usage sheets, error strings, log output, and all
other API-controlled values enter the document through text nodes or safe
attribute assignments with fixed semantics. The dashboard does not introduce
`innerHTML` or another markup sink. Device graphics remain static templates or
CSS shapes whose kind is selected from a fixed allowlist.

## Failure and edge states

If refresh or SSE fails, the dashboard keeps the last successful fleet snapshot
visible and states how old it is. It does not replace useful data with a full
page error.

Empty fleets teach the operator how a worker appears. Empty queues and healthy
attention states reclaim space instead of rendering empty containers. Unknown
future device or job states use a neutral visible fallback and their raw state
word.

Long IDs, selectors, commands, quarantine reasons, labels, and usage sheets are
bounded or wrapped without widening the page. Large fleets switch to the
matrix presentation instead of shrinking host bays below readable dimensions.

Destructive or fleet-wide actions require confirmation that names the exact
target and consequence. A failed action leaves the current state visible and
reports the server's error beside the action that failed.

## Responsive and accessible behavior

At wide desktop sizes, the fleet and attention region share the viewport when
incidents exist, and workspace facts sit beside logs. At narrower desktop and
tablet sizes, the attention region moves above the machine room when incidents
exist, and workspaces use stacked sections. On phones, host bays become a
vertical inventory and workspace Details and Output become tabs. No critical
action is hidden solely because the viewport is narrow.

All interactive surfaces are native buttons, links, inputs, or dialogs where
appropriate. Focus enters a workspace at its heading and returns to the entity
that opened it. Escape closes the workspace only when no nested confirmation
owns Escape. Visible focus, logical tab order, semantic headings, live-region
announcements, and non-color state cues are required.

## Testing

Keep all existing dashboard security tests. Add source or DOM tests that pin:

- the absence of markup sinks and `localStorage`;
- the single allowed `sessionStorage` key;
- admin-token prompt, use, disposal, and non-copying behavior;
- static allowlisted device graphics;
- reduced-motion handling;
- browser-history view identifiers that never contain credentials.

Add browser-level interaction tests for:

- opening and closing each workspace through pointer, keyboard, Back, and
  Escape;
- focus placement and restoration;
- preserving overview filters, sorting, and scroll position;
- following logs from the initial live edge;
- leaving the live edge without scroll jumps;
- Jump to live and automatic follow restoration at the bottom;
- buffered appends and the retained-output cap;
- stream completion, interruption, explicit reconnection, and cleanup when
  switching jobs;
- state refreshes that update job facts without rebuilding the log viewer;
- filtering, sorting, incident changes, and reduced-motion fallbacks;
- destructive-action confirmation and failed-action feedback.

Test representative visual states at desktop, tablet, and phone widths:

- no devices;
- one host with one device;
- a large fleet that requires matrix mode;
- long identifiers and commands;
- mixed ready, occupied, disconnected, and unhealthy states;
- several simultaneous incidents;
- a long queue and a long-wait job;
- a device with stale or missing label ages;
- light and dark system preferences.

Run the complete Go test suite after asset changes because the dashboard tests
exercise the bytes served by the embedded handler.

## Scope exclusions

This redesign does not add controller-side event retention, log indexing,
server-side log search, metrics collection, utilization time series, a new
authentication model, or a frontend framework.

It does not invent topology data the controller does not have. Host grouping is
derived from device host fields; highlighting job eligibility uses only
selectors the client can evaluate correctly or data the server already
provides.

It does not add decorative ambient animation. Any future visualization must
identify the operational question it answers before it enters the dashboard.
