# SDR (reference tenant 2, first slice)

An outbound cadence on WhatsApp, rebuilt from the legacy SDR:

1. The CRM posts a new lead to the webhook trigger `crm-novo-lead`
   (`{leadId, phone, name, optIn}`, signed). The cadence runs in the
   lead's WhatsApp conversation (`wa:<phone>`).
2. First contact: an approved template, A/B tested (`primeira-mensagem`:
   control vs social proof, judged on replies within 7 days).
3. Wait 48h for the reply. A reply gets an answer in free text (the lead
   just wrote: the 24h window is open) and ends the cadence; silence gets
   the follow-up template, then 72h and the nurture template.

The platform enforces the channel rules from `plugins.yaml`: no message
without the CRM's opt-in (`consent`), no free text outside 24h after the
lead's last message (`window`): the RFC-0001 checklist item, verified by
`evals/cadencia.evalsuite.yaml` on a simulated clock.

## Run it

```bash
make build
./bin/tessera test testdata/cadencia                      # the cadence, on a simulated clock, no model calls

export TESSERA_API_TOKEN=$(openssl rand -hex 32)  # the /v1 API and console (32+ characters)
export TESSERA_SECRET_CRM_HOOK=...            # the CRM signs its webhook with it
export TESSERA_SECRET_WHATSAPP_TOKEN=...      # Meta access token (Graph API)
export TESSERA_SECRET_WHATSAPP_APP_SECRET=... # Meta app secret: checks X-Hub-Signature-256
export TESSERA_WHATSAPP_VERIFY_TOKEN=...      # the verify token you set in Meta's webhook config
./bin/tessera serve testdata/cadencia --namespace sdr --db sdr.db --whatsapp
```

In Meta's app settings, point the webhook to
`https://<your host>/channels/whatsapp/webhook` and subscribe to
`messages`. Put your phone number ID in `plugins.yaml` (`baseUrl`) and
create the templates `outbound_first_contact`, `outbound_prova_social`,
`followup_outbound` and `nurture_reengagement` in WhatsApp Manager.

Numbers: `tessera metrics funnel testdata/cadencia --namespace sdr --funnel
prospeccao --db sdr.db` and `tessera metrics experiment ... --experiment
primeira-mensagem`.

## Not yet

Qualification (NEADS/SPIN), CRM (the CRM) and scheduling plugins, the
team's reports, shadow mode against the legacy platform's traffic (#14).
