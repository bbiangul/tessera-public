# plugin-001: documents from a plugin (SharePoint)

- **Date:** 2026-09-27.
- **Pre-registration:** `plugin-001-preregistration.md` (commit 5c13a00,
  before the code).
- **Code:** d111898.
- **Data:** `bench/results/plugin-001/`.
- **Spend:** about $0.02 for the agents and $0.52 for the judge.

## What was built

**A KnowledgeBase source can be a plugin:**
- `list`, a read that returns the documents;
- `fetch`, called for each listed document, with the listed document as
  `inputs`;
- `refresh`, default 24h.

**Deviation:** the pre-registration wrote `${ item.id }`; it is
`${ inputs.id }`. Adding an `item` variable would have widened what
every other expression may read.

**Reading and refresh:**
- Files and plugin documents are indexed together, and the index is
  swapped in whole.
- Plugin sources are read when the agent loads, bounded to 30 s. Loading
  never fails because of the network.
- After that, `Tick` reads each source when it is due.
- A failed list keeps its documents; a failed fetch keeps that
  document's previous version. Both are retried within 5 minutes, and the
  errors are returned by `Tick`.
- A document with an unchanged `modified` is not fetched again.
- Unsupported file types are skipped.

**HTTP bindings: `response: bytes`**, for reads only:
- the body comes back as `{base64, contentType, size}`;
- up to 3 redirects are followed, each to a host in `egress.hosts`,
  which now accepts `"*.domain"` patterns;
- the private-address refusal holds on every hop;
- `Authorization` is not sent to another host (Go's rule, pinned by a
  test);
- JSON bindings still follow no redirects.

**`tessera add sharepoint --tenant … --site …`** writes a Microsoft Graph
manifest:
- OAuth2 client credentials;
- `list-files`: a folder's files, subfolders excluded;
- `download`: `/content`, which redirects to the tenant's
  `*.sharepoint.com`.

## Result

| # | Result |
| --- | --- |
| G1 | **Supported.** knowledge-001's 30 questions, the documents coming only from a sandboxed plugin (7 as text, 2 as base64 DOCX), 3 repeats: answerable 54 of 54, staff 6 of 6, 0 leaks, 0 injections obeyed, 0 unsupported facts. |
| G2 | **Supported.** Refresh unit tests: load; not due, not called; a change served in its new version; a removal gone; an unchanged document not fetched again; a failed list keeps both documents and retries after 5 minutes, not 4; a failed fetch keeps the previous version. Mutations caught for every rule. |
| G3 | **Supported.** Bytes come back as base64. A redirect to another allowed host carries no token. A redirect outside egress is refused, and so is a 4th hop (exactly 4 requests per attempt). A JSON binding refuses redirects. A private address is refused. Mutations caught for every rule. The redirect-limit test first accepted any error and let the mutation through; it now counts the requests. |
| G4 | **Supported.** The generated manifest against a Graph stand-in (token, children with a subfolder, `/content` 302 to `contoso.sharepoint.com`): the PDF and the DOCX were listed, downloaded and indexed, with their sections. The download host never saw the token. |
| G5 | **Supported:** `make ci`, `make lint-extra`. 12 mutations caught in all. |

**Not validated:** real SharePoint. No tenant or credentials were used.
A live check needs the owner's Entra app registration (Sites.Selected
granted to one site, or Sites.Read.All). Added to #56.

## Limits

- The folder is one path segment: a nested path such as "A/B" is escaped
  as one segment. Graph may need a different path for it; not tested.
- No paging: a folder with more files than one Graph page (200 by
  default) lists only the first page.
- Google Drive is not built: its service-account (JWT) auth is missing.
