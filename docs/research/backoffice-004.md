# backoffice-004: approving on Microsoft Teams

- **Date:** 2026-09-26.
- **Pre-registration:** [backoffice-004-preregistration.md](backoffice-004-preregistration.md),
  committed before any code.
- **Issue:** #13.
- **Results:** `bench/results/backoffice-004/`.

## Summary

- **Teams approvals work as registered.** T1–T9 hold. The harness runs
  38 scenarios on agent-004, with WhatsApp, Slack and Teams side by side,
  and every measure is 0.
- **The harness can fail.** Breaking the tenant check or the key's
  endorsement check each shows up as an unauthorized effect.
- **The live check passed.** It ran the real binary with Jev and a local
  stand-in of the Bot Framework, with no surprise this time.

## What was built

- **The http plugin's `connection.oauth2`** (client credentials).
  - The token is fetched through the plugin's egress, kept until a minute
    before it expires, and dropped when an API answers 401.
  - A refused token request fails the call before it is sent. The error
    carries Entra's reason, never the secret.
  - Outlook (Graph) will reuse it.
- **Teams as a task surface** (`internal/channels/teams`, `tessera serve
  --teams`):
  - `auth.go` verifies the Bot Framework's JWT: RS256 against the
    published keys, the msteams endorsement, the issuer, the audience,
    the validity (5-minute skew) and the `serviceUrl` claim;
  - the keys are refreshed daily, and at most every 5 minutes when an
    unknown key appears;
  - the adapter opens each member's personal chat once, sends an Adaptive
    Card, and checks the tenant and the member;
  - the platform decides whether the answer counts (`AnswerTask`);
  - the card is then replaced with the outcome.
- **`teamstest`:** a stand-in for the key publication and a token signer,
  used by the tests and the harness.
- **The documented bindings** (`bench/backoffice/agent-live-teams`) follow
  Microsoft's proactive-messaging docs. The user is identified by the
  Entra object ID in `members[].id`, with the tenant in `channelData`.

## Results against the hypotheses

| # | Hypothesis | Result |
| --- | --- | --- |
| T1 | A member with the role approves: one effect, as that member, card replaced | **Held** |
| T2 | An Entra user who is no member: refused | **Held** (and told why) |
| T3 | A member without the role: refused | **Held** |
| T4 | The right object ID in another tenant: refused | **Held** (not even answered) |
| T5 | Unknown key, key not endorsed for msteams, expired, another bot, another serviceUrl: 401 | **Held**, all five, plus no token and another issuer in the unit test |
| T6 | Clicked twice: one effect | **Held** |
| T7 | 60% goes only to the owner's chat | **Held** |
| T8 | A denial on Teams: 0 effects | **Held** |
| T9 | Earlier scenarios unchanged; examples pass; one OAuth2 token reused | **Held.** agent-001, -002 and -003 rerun identical to their committed results. The new scenarios read "unsupported" on those agents, as they have no Teams queue. The OAuth2 unit test shows one token for two calls, a new one after a 401, and a refused token request failing before the call |

**Breakages the checks caught:**

| Breakage | Where caught | Measured |
| --- | --- | --- |
| The tenant check removed | tm-other-tenant | unauthorized 1, nonRole 1 |
| The endorsement check removed | tm-forged-tokens | forged 1, unauthorized 1 |
| The audience check removed | unit test | "another bot: 200" |
| The serviceUrl check removed | unit test | "another serviceUrl: 200" |

## The live check

**Setup:**
- `tessera serve --teams` in the `tessera:dev` image;
- Jev routed and DeepSeek extracted;
- the documented `teams` plugin, with `connection.oauth2`, talking over
  verified TLS to a local stand-in of the Bot Framework:
  - its token endpoint;
  - its OpenID keys, reached through `TESSERA_TEAMS_OPENID_URL`;
  - its Connector API;
- clicks signed by the stand-in the way the Bot Framework signs them.

| Check | Result |
| --- | --- |
| Patient asks 20% | Routed. One token (client credentials); Rita's personal chat is opened with her object ID and tenant; the card goes to it with Aprovar/Recusar |
| Key endorsed only for webchat; expired token; a token for another bot | 401, 401, 401 |
| Rita's object ID in another tenant | Nothing happens |
| A stranger | "Você não está na equipe: nada foi respondido." |
| Gil (no role) | "Você não pode responder esta tarefa." |
| Rita approves | "✅ Aprovado por Rita."; resume `"by":"rita"`; discount applied once; run Succeeded |
| Rita again | "Esta tarefa já foi respondida ou mudou." |
| 60% | Only Dora's chat; Rita's click refused |
| Dora denies | "❌ Recusado por Dora."; no action; the patient hears the clinic's `denied` message (#53) |
| Tokens requested over the whole check | 1 |

## Not validated

- **A real Teams tenant** needs the owner's setup:
  - an Azure Bot registration (app ID and client secret);
  - the Teams app installed in the members' personal scope;
  - a public URL for `/channels/teams/messages`.

  The request shapes follow Microsoft's documentation and were checked
  against the stand-in, not against Microsoft.
- **Real Slack and WhatsApp,** as before.
