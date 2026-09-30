# Adversarial review of the approval surfaces (2026-09-26)

**What was reviewed.** Commits b0949f2..40709ce: the WhatsApp template
fallback, Slack, Teams, e-mail, the #53 fix, `connection.oauth2`, and
their admission rules.

**How.** Three independent reviewers each took one area:
- how channel clicks are authenticated;
- task links, one-time codes, e-mail and OAuth2;
- denial semantics and admission.

A finding counted only if it came with a test that fails on 40709ce. All
21 were re-run and confirmed before fixing. The reviewers' tests are kept
as regression tests (`advrev_test.go` in each package). Where a test only
logged the defect, it was rewritten to assert the fixed behavior.

**Summary.** No reviewer found a way to forge an answer, answer as
another member, or skip the one-time code. Many other attacks were tried
and held up; each reviewer's list is in its section below. The one
high-severity defect was a wrong meaning, not a forgery.

## Findings and fixes

| # | Severity | Defect | Fix |
| --- | --- | --- | --- |
| 1 | **High** | `escalate` with `task: approve` in an action's `onError` asked "did it happen?". The channels rendered it as Approve, so Approve reported a discount that never ran as done, and the user heard "aprovado" | Admission refuses `task: approve` there, with the fix "use `task: answer`". The runtime never presents that question as an approval (`escalateEffect` uses kind `answer`) |
| 2 | Medium | Code rationing was per link token. Each notification and each e-mail is a new token, so the allowance multiplied (12 codes and 60 guesses an hour, where 3 codes were designed) | Codes and allowances are keyed by task and member: 3 per member and 10 per task per hour, whatever the link |
| 3 | Medium | Teams signing-key fetch had no backoff and held the lock. An unsigned request with an unknown kid made one fetch per request, and legitimate clicks waited | At most one fetch per minute, successful or not. The fetch runs outside the lock and one at a time, and known keys never wait |
| 4 | Medium | A task forwarded or escalated from an open queue to a queue with roles stayed open to anyone, and never reached that queue's channel | Once any reached queue has roles, only their holders answer, and the channels deliver it |
| 5 | Medium | A corrected member Teams ID kept sending cards to the old person's chat (cache keyed by member only) | The chat cache is keyed by member and Entra ID |
| 6 | Low-Med | A second approver's identical answer, during a retried execution, was shown as theirs while the journal recorded the first | `sameReply` compares `by`; the second approver sees "já foi respondida" |
| 7 | Low-Med | Tasks of older runs were never delivered once more than 500 newer runs waited | `ChannelTasks` pages through every waiting run |
| 8 | Low-Med | Free-text approvals were read from the first word: "sim, mas só 5%" and "ok, não aprovo" approved | Approvals need the whole answer to be one yes word. "Did it happen?" keeps the leading-word reading, which an earlier review settled |
| 9 | Low | A database error at click time showed "não está mais aberta" and removed the buttons of an open task | Only a missing row says so; other errors are logged and the buttons stay (WhatsApp: "tente de novo") |
| 10 | Low | Teams could not work in a sovereign cloud: the issuer was hard-coded | The issuer is the one the OpenID configuration names |
| 11 | Low | A valid code was used up by a request refused for a missing decision | The form is checked before the code. After a failed recording the page says to ask for a new code |
| 12 | Low | Code bookkeeping was never pruned | Swept at most once a minute |
| 13 | Low | A late 401 from an older request dropped a freshly fetched OAuth2 token | Only the refused token is dropped |
| 14 | Low | A refresh token the provider rotated was thrown away, so the next refresh failed | Kept in memory. A restart uses the secret again |
| 15 | Low | A token endpoint echoing the request could put the client secret or refresh token into errors and logs | Credentials, plain and URL-encoded, are redacted from every token error |
| 16 | Low | `error_description` was unbounded and could span lines | One line, at most 300 bytes |
| 17 | Low | `expires_in` as a string failed, and a missing one refetched every call | A number or a numeric string. The default is 5 minutes |
| 18 | Low | `oauth2.tokenUrl` on a private or metadata address was admitted | Refused, as for `baseUrl` |
| 19 | Low | A queue's channel whose role holders have no address there was admitted silently | Warning `NO_ADDRESS` |
| 20 | Info | An e-mail that is not exactly one address (`a,b@x`) was written verbatim into To | `mailmsg.Address` refuses it, admission checks members' e-mails with `net/mail`, and `TESSERA_EMAIL_FROM` is checked at startup |
| 21 | Info | Long subjects were one unfolded line (over 998 bytes) | Encoded words are folded one per line |

## Open, not fixed here

- **A captured Teams token.** It proves only that the request comes from
  the Bot Framework and names the serviceUrl. The member, tenant and
  button come from the unsigned body, so whoever captures a token (for
  example from the logs of a TLS-terminating proxy) can forge clicks until
  it expires. The Bot Framework has no per-activity signature, so this is
  a limit of the platform. Keep Authorization headers out of proxy logs.
- **Shutdown order in `serve`.** The ticker goroutine is not waited for
  before the adapters close. No panic could be reproduced; the visible
  effect was finding 9, which is now fixed.
- **Slack Enterprise Grid.** `team.id` may be the org rather than the
  workspace. Valid clicks would then be refused; this is never a bypass.
  Not reproduced.
- **`onTimeout.answer`.** A string `"sim"` denies, and `true` approves
  every timed-out task. Both are by design, but admission could warn.
- **A denial outside WhatsApp's 24h window** is not delivered to the user
  (`WINDOW_CLOSED`), the same as the approved path's output today.
