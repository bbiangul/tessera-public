# backoffice-004: approving on Microsoft Teams. Pre-registration

- **Date:** 2026-09-26, before any of this is built.
- **RFC:** RFC-0006, the approval surfaces. Teams is the channel after
  Slack, in the RFC's order.
- **Issue:** #13.

## What will be built

1. **`HumanQueue.spec.channels: [teams]`,** plus the `Team` members'
   `teams`: their Microsoft Entra object ID (a GUID).
2. **Presenting:** `tessera serve --teams` sends each approval task of
   such a queue to the member's personal chat with the bot, as an Adaptive
   Card with Aprovar / Recusar (`Action.Submit`). It goes through a plugin
   named `teams` with three actions, all required by admission:
   - `open-dm {user}`: creates or gets the personal conversation
     (Bot Framework `POST /v3/conversations`), returning its id;
   - `post-task {conversation, text, buttons}`: sends the card;
   - `update-task {conversation, activity, text}`: replaces the card with
     the outcome.
3. **Calling the Bot Framework** needs an OAuth2 token (client
   credentials). The http plugin gains `connection.oauth2`: `tokenUrl`,
   `clientId` and `clientSecret` secrets, and `scope`. The token is
   fetched through the plugin's own egress and kept until shortly before
   it expires. The e-mail surfaces (Microsoft Graph) will reuse it.
4. **Receiving:** Teams delivers a click to `/channels/teams/messages` as
   an activity with a Bearer JWT. The click is an answer only when:
   - the JWT is RS256-signed by a key from the Bot Framework's published
     keys (OpenID configuration);
   - that key is endorsed for the `msteams` channel;
   - the issuer is `https://api.botframework.com`;
   - the audience is this bot's app ID;
   - the token is within its validity;
   - the `serviceUrl` claim matches the activity's;
   - the activity's tenant is the configured one (`TESSERA_TEAMS_TENANT_ID`);
   - the sender's Entra object ID is a member's, that member holds a role
     of the queues the task reached, and the hash is still the task's.

   The platform decides whether the answer counts (`runtime.AnswerTask`),
   as for WhatsApp and Slack. The request is acknowledged at once; the
   answer is recorded after.

## Measure

**The backoffice harness gains Teams scenarios on
`bench/backoffice/agent-004`:** agent-003 with `teams` added to every
queue and an Entra ID on every member. The scenarios use:
- a local key pair published as the harness's Bot Framework keys, and
  tokens signed with it;
- a fake `teams` plugin that records the calls;
- the real adapter.

## Hypotheses

| # | Hypothesis |
| --- | --- |
| T1 | A member with the role clicks Aprovar: one effect, recorded as that member; the card is replaced with the outcome |
| T2 | An Entra user who is no member: refused, no effect |
| T3 | A member without the role: refused |
| T4 | The right object ID in another tenant: refused, no effect |
| T5 | Each of these gets HTTP 401 and no effect: a token signed by an unknown key; a key not endorsed for msteams; an expired token; a token for another bot (audience); a `serviceUrl` claim that differs from the activity's |
| T6 | Clicked twice: one effect |
| T7 | A 60% discount goes only to the owner's chat |
| T8 | Rita clicks Recusar: 0 effects (#53) |
| T9 | Every earlier scenario stays 0 on agent-004, every example passes, and the http plugin reuses one OAuth2 token across calls |

**After:** a live check through `tessera serve --teams` in the
`tessera:dev` image (as in backoffice-003), against a local stand-in of
the Bot Framework: token endpoint, conversations, and OpenID keys. The
stand-in's key replaces Microsoft's only through the OpenID URL setting.
A real Teams tenant needs the owner's Azure bot registration; until
then, the report says so.
