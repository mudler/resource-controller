# Dashboard fake-fleet preview harness design

## Goal

Run the accepted dashboard redesign against a disposable, realistic fake fleet
so its layout, workspaces, actions, activity updates, and log behavior can be
reviewed in a browser before the feature branch is merged.

## Scope

The harness is preview-only. It lives under the ignored `.superpowers/`
directory in the preserved feature worktree and is never committed with the
product. It does not modify a controller database, contact workers, or use real
credentials.

The server binds to `0.0.0.0` and prints a LAN URL. It serves the real dashboard
and vendored Anime.js files from the accepted feature worktree.

## Fake fleet

The preview contains three hosts and eight devices with enough variation to
exercise the interface:

- free H100 and A100 devices;
- active training and evaluation jobs with different elapsed times;
- one disconnected device;
- one unhealthy device with a quarantine reason;
- two queued jobs with selectors, priorities, and different waits;
- labels with declared/detected provenance and freshness;
- host usage guidance and recent completed/failed jobs.

## Preview API

The disposable server implements only routes the dashboard calls:

- `GET /` and `GET /dashboard/anime.umd.min.js`;
- `GET /v1/state`;
- `GET /v1/whoami`;
- `GET /v1/events` as an SSE stream;
- device describe routes;
- job detail and streaming-log routes;
- kill, clear, and retire actions as in-memory fake state changes.

The token gate accepts the value `preview`. Admin prompts also accept
`preview`. No supplied token is written to server logs.

## Dynamic behavior

The server periodically advances job elapsed times and emits fake activity
events. One log endpoint streams bounded sample output in short chunks. Action
requests update only the in-memory fake snapshot and trigger an SSE refresh.

The data resets whenever the preview server restarts.

## Safety and verification

The harness uses no production controller package and writes no database. It
serves files only from explicit paths in the feature worktree. Unknown paths
return 404.

Verification checks that the root dashboard, local Anime.js asset, authenticated
fake state, device describe, job detail, SSE, and streaming log routes respond
successfully before the LAN URL is handed off.
