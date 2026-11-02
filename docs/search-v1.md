# MasterDrive G1: bounded, opt-in federated filename search

Status: **source-level experimental design and implementation**. This is not an indexed, recursive, full-text, or automatically scheduled crawler. No live multi-device qualification has been completed.

## Explicit search boundaries

The owner must already have exported each directory and granted the caller the relevant share. A search request names exactly which shares to query; neither the agent nor controller discovers a new device or enumerates unrelated volumes. The source agent checks the same bearer credential and share grant as ordinary listing. It reads **immediate children only** of the requested folder and never opens file contents for indexing.

| Limit | Present G1 behavior |
| --- | --- |
| Share selection | Caller provides 1–8 explicit `host-id:share-id` targets |
| Fanout | At most 3 source requests concurrently, under one 20-second deadline |
| Source work | At most 256 immediate directory entries examined in a call; bounded 0–10,000 offset |
| Results | At most 50 names per target/call, no recursive descent or disk-content scan |
| Continuation | Per-source `next_offset`; continue **one target at a time** |
| Errors | Each source reports an error code; `complete` is false if any source fails, paginates, or hits the offset cap |

## Endpoints

Source agent (bearer authorized):

`GET /v1/search?share=docs&path=folder&q=report&limit=25&offset=0`

Response shape:

```json
{
  "share": "docs", "path": "folder", "query": "report",
  "scope": "immediate_children", "entries": [], "scanned": 20,
  "next_offset": null, "truncated": false
}
```

Controller (local browser session):

`GET /api/v1/search?q=report&target=<host-id>:docs&target=<another-host-id>:photos`

Returns `query`, `path`, `scope: "immediate_children"`, `complete`, and a `sources` list with `host_id`, `share_id`, `entries`, `scanned`, `next_offset`, `truncated`, and optional `error`. No peer credential, source OS path, or arbitrary URL is included. For a continuation, issue one `target` with `offset=<that source's next_offset>`; this avoids mixing independent peer cursors.

The controller proxies only registered host origins, reuses the existing protected peer HTTP client, holds credentials in memory, and validates a strict, size-limited response before displaying results. An unregistered host or missing token never triggers remote probing. On per-share grants, ungranted shares return generic `404` from the agent. A partial source failure is not represented as a complete search result.

## Caveats and next gates

- Directory entry order and offset boundaries are not snapshots. Concurrent adds/deletes can cause misses or duplicates. A stable indexed search requires explicit scanning schedules, filesystem identity, snapshot/version invalidation, privacy policies, user permission models, and stale/offline provenance.
- A large directory may exceed the 10,000-position offset cap. When continuation is impossible, `truncated: true` keeps the result visibly incomplete. This is an intentional resource ceiling.
- Current matching is Unicode lowercasing + substring matching on names, not locale-aware fuzzy search, file content inspection, or global metadata indexing.
- The G1 browser UI now exposes explicit host/share/folder selection, immediate-folder matches, partial/offline results, and paginated continuation. The UI has JavaScript regressions for stale requests and non-advancing cursors; browser-rendered and real multi-device qualification remain outstanding.
- Search terms are currently carried in GET query parameters to the local controller and to each selected source agent. Local reverse proxies, browser history, and HTTP access logs may retain these names. Keep queries non-sensitive during testing and do not enable query-string logging; a future privacy review must establish redaction or a different request transport before sensitive use.
- Security qualification: wrong/absent token on every search route, inaccessible/grant-excluded share, hostile names and symlink escape attempts, thousands of entries, request fanout cancellation, peer redirects, malformed/oversized response, Windows junctions, offline hosts, and no background scan when idle.

Primary references: [Go `File.ReadDir`](https://pkg.go.dev/os#File.ReadDir) (bounded `n`), [Go `Request.Context`](https://pkg.go.dev/net/http#Request.Context) (cancellation), and [OWASP API4: Unrestricted Resource Consumption](https://owasp.org/API-Security/editions/2023/en/0xa4-unrestricted-resource-consumption/).
