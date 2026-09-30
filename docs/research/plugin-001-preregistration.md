# plugin-001: documents from a plugin (SharePoint). Pre-registration

- **Date:** 2026-09-27, before any of this is built.
- **RFC:** RFC-0008, part A: "fontes por plugin (SharePoint, Drive)".

## What will be built

1. **A KnowledgeBase source can be a plugin action.**

   ```yaml
   sources:
     - list:  { act: sharepoint/list-files, with: { folder: "Politicas" } }
       fetch: { act: sharepoint/download, with: { id: "${ item.id }" } }   # optional
       refresh: 24h
   ```

   - **`list`** is a read. It returns documents
     `{name, id?, modified?, text?, base64?}`.
   - **`fetch`**, when set, is called for each listed document without
     content. It returns `{base64}` or `{text}`, or the bytes body of an
     HTTP binding.
   - **The file type** comes from the name's extension (`.md`, `.txt`,
     `.pdf`, `.docx`). Other types are skipped and counted.
   - **Text** goes through the same extraction and splitter as files.
2. **Refresh.** Plugin sources are read when the agent loads, then again
   every `refresh` (default 24h) by the host's tick, which `serve` runs.
   - A new index is built and swapped in whole, so answers never see a
     half-built one.
   - **A failed list keeps that source's previous documents.** A failed
     fetch keeps the previous version of that document. A failure never
     empties a base.
   - Failures are logged with the action's error.
   - A source that fails when the agent loads starts empty and is retried
     on the next tick, after 5 minutes at most. Loading never fails
     because of the network.
3. **`response: bytes` on an HTTP binding** (reads only). The body is
   returned as `{base64, contentType, size}`, within the existing size
   limit.
   - Redirects are followed, at most 3, only for these bindings. Graph's
     `/content` answers with a redirect.
   - Each hop still passes the private-address refusal.
   - The `Authorization` header is not sent to another host. Go's client
     drops it; a test pins it.
4. **`tessera add sharepoint`** writes a PluginManifest for Microsoft
   Graph:
   - **auth:** OAuth2 client credentials (app-only; the secrets are the
     owner's);
   - **`list-files`:** the children of a folder of a site's drive;
   - **`download`:** an item's content, with `response: bytes`.

   Google Drive is a later slice: its service-account auth (JWT) is not
   built.

## Measure

| Part | What runs |
| --- | --- |
| **Parity (live)** | knowledge-001's set, 3 repeats. The same documents come from a sandboxed plugin: `list` returns the 9 names; `fetch` returns 7 as text and 2 as base64 DOCX (`convenios`, `interno-descontos`). Same judge as pdf-001. |
| **Refresh (unit, deterministic)** | A document changed between two reads is served in its new version after a tick. One removed is no longer retrieved. A failed list, or a failed fetch, keeps the previous version. Not due, nothing is called. |
| **Bytes and redirects (unit)** | A binary body comes back as base64. A redirect to another host is followed without `Authorization`. A redirect to a private address is refused. More than 3 redirects is an error. A JSON binding still does not follow redirects. |
| **SharePoint (integration test)** | `tessera add sharepoint`'s manifest, against a local Graph stand-in (token, children, a `/content` 302 to a download URL): a PDF and a DOCX listed, downloaded and indexed, and their passages found. |

## Hypotheses

| # | Hypothesis | Supported if |
| --- | --- | --- |
| G1 | A plugin source works like files | answerable correct ≥ 51 of 54; 0 staff-only leaks; 0 injections obeyed |
| G2 | Refresh is safe | every refresh unit test passes, with a mutation caught for each rule |
| G3 | Bytes and redirects are safe | every bytes/redirect unit test passes, with a mutation caught for each rule |
| G4 | SharePoint end to end | the integration test passes against the stand-in |
| G5 | Nothing else changes | `make ci` and `make lint-extra` pass |

**Not claimed:** anything about real SharePoint. No tenant or
credentials are used; a live check needs the owner's app registration.

**Budget:** $1.5.
