package admission

import (
	"fmt"
	"net/mail"
	"regexp"
	"strings"

	agents "github.com/bbiangul/tessera/api/agents/v1alpha1"
	plugins "github.com/bbiangul/tessera/api/plugins/v1alpha1"
)

var (
	slackID = regexp.MustCompile(`^[UW][A-Z0-9]{2,}$`)
	entraID = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
)

// checkTeams checks Teams and the queues' roles (RFC-0006): member ids
// are unique in a namespace, the verify action sends a message, and a
// queue's roles are held by some member.
func (c *Checker) checkTeams(docs []Doc, pluginsByName map[string]*plugins.PluginManifest) []Issue {
	var issues []Issue
	roles := map[string]map[string]bool{}  // namespace -> roles held
	verifies := map[string]bool{}          // namespaces whose Team sends codes
	teamOf := map[string][]agents.Member{} // namespace -> members
	members := map[string]map[string]bool{}
	for _, d := range docs {
		if d.Team == nil {
			continue
		}
		ns := d.Team.Metadata.Namespace
		if roles[ns] == nil {
			roles[ns], members[ns] = map[string]bool{}, map[string]bool{}
		}
		add := func(code, path, msg, fix string) {
			issues = append(issues, Issue{Severity: SeverityError, Code: code, File: d.File, Resource: d.ID(), Path: path, Message: msg, Fix: fix})
		}
		teamOf[ns] = append(teamOf[ns], d.Team.Spec.Members...)
		for i, m := range d.Team.Spec.Members {
			if m.WhatsApp != "" && (len(m.WhatsApp) < 8 || len(m.WhatsApp) > 15) {
				add("INVALID_FIELD", fmt.Sprintf("spec.members[%d].whatsapp", i), "an E.164 number has 8 to 15 digits", "digits only, with the country code: 5511900000001")
			}
			if m.Slack != "" && !slackID.MatchString(m.Slack) {
				add("INVALID_FIELD", fmt.Sprintf("spec.members[%d].slack", i), "a Slack user ID looks like U0123ABCD", "copy it from the member's Slack profile: ⋮ → Copy member ID")
			}
			if m.Teams != "" && !entraID.MatchString(m.Teams) {
				add("INVALID_FIELD", fmt.Sprintf("spec.members[%d].teams", i), "a Microsoft Entra object ID is a GUID", "copy the member's Object ID from Entra ID → Users")
			}
			if m.Email != "" && !oneAddress(m.Email) {
				add("INVALID_FIELD", fmt.Sprintf("spec.members[%d].email", i), "not an e-mail address", "")
			}
			if members[ns][m.ID] {
				add("DUPLICATE", fmt.Sprintf("spec.members[%d].id", i), fmt.Sprintf("member %q is listed twice in namespace %s", m.ID, ns), "")
			}
			members[ns][m.ID] = true
			for _, r := range m.Roles {
				roles[ns][r] = true
			}
		}
		if v := d.Team.Spec.Verify; v != nil {
			verifies[ns] = true
			pl, name := splitAct(v.Act)
			p := pluginsByName[pl]
			switch {
			case p == nil:
				add("UNKNOWN_PLUGIN", "spec.verify.act", fmt.Sprintf("no PluginManifest named %q", pl), "")
			case p.Spec.Actions[name].Effect == "":
				add("UNKNOWN_ACTION", "spec.verify.act", fmt.Sprintf("plugin %s has no action %q", pl, name), "")
			case p.Spec.Actions[name].Effect != plugins.EffectExternalMessage:
				add("INVALID_FIELD", "spec.verify.act", v.Act+" must be an external-message action: it sends the member a code", "")
			}
			if v.With != nil {
				if _, err := c.expr.CompileValue(v.With); err != nil {
					add("INVALID_EXPR", "spec.verify.with", err.Error(), "")
				}
			}
		}
	}
	channelPlugin := map[string]string{} // namespace -> the Agent's channel plugin
	for _, d := range docs {
		if d.Agent != nil {
			channelPlugin[d.Agent.Metadata.Namespace] = d.Agent.Spec.Channel
		}
	}
	for _, d := range docs {
		if d.Queue == nil {
			continue
		}
		for i, ch := range d.Queue.Spec.Channels {
			path := fmt.Sprintf("spec.channels[%d]", i)
			add := func(code, msg, fix string) {
				issues = append(issues, Issue{Severity: SeverityError, Code: code, File: d.File, Resource: d.ID(), Path: path, Message: msg, Fix: fix})
			}
			if ch != "whatsapp" && ch != "slack" && ch != "teams" && ch != "email" {
				add("INVALID_FIELD", fmt.Sprintf("unknown channel %q", ch), "whatsapp, slack, teams or email")
				continue
			}
			if len(d.Queue.Spec.Roles) == 0 {
				add("MISSING_FIELD", "a channel sends tasks to the queue's members: the queue needs roles", "")
			} else if !reachable(teamOf[d.Queue.Metadata.Namespace], d.Queue.Spec.Roles, ch) {
				issues = append(issues, Issue{Severity: SeverityWarning, Code: "NO_ADDRESS", File: d.File, Resource: d.ID(), Path: path,
					Message: fmt.Sprintf("no member holding %v has a %s address: this channel reaches nobody", d.Queue.Spec.Roles, ch), Fix: "add the members' " + ch + " to the Team"})
			}
			if ch == "email" {
				if pl := pluginsByName["email"]; pl == nil || pl.Spec.Actions["send-task"].Effect == "" {
					add("MISSING_FIELD", "tasks by e-mail need a plugin named email with the send-task action", "send-task {to, subject, text, link, raw} sends the e-mail (Graph sendMail, or Gmail with raw)")
				}
				if !verifies[d.Queue.Metadata.Namespace] {
					add("MISSING_FIELD", "tasks by e-mail are answered through their link with a one-time code: the Team needs verify", "add spec.verify to the Team (a code by WhatsApp, SMS or e-mail)")
				}
				continue
			}
			if ch == "teams" {
				pl := pluginsByName["teams"]
				for _, a := range []string{"open-dm", "post-task", "update-task"} {
					if pl == nil || pl.Spec.Actions[a].Effect == "" {
						add("MISSING_FIELD", "tasks on Teams need a plugin named teams with the "+a+" action", "open-dm {user, tenant, bot} returns the personal chat's id; post-task {conversation, text, buttons} sends the card; update-task {conversation, activity, text} replaces it with the outcome")
					}
				}
				continue
			}
			if ch == "slack" {
				pl := pluginsByName["slack"]
				for _, a := range []string{"post-task", "update-task"} {
					if pl == nil || pl.Spec.Actions[a].Effect == "" {
						add("MISSING_FIELD", "tasks on Slack need a plugin named slack with the "+a+" action", "post-task {to, text, buttons} posts to the member's DM; update-task {channel, ts, text} replaces the buttons with the outcome")
					}
				}
				continue
			}
			pl := pluginsByName[channelPlugin[d.Queue.Metadata.Namespace]]
			if pl == nil || pl.Spec.Actions["send-buttons"].Effect == "" {
				add("MISSING_FIELD", "the Agent's channel plugin needs a send-buttons action {to, text, buttons} to present tasks", "declare it in the WhatsApp plugin (an interactive message)")
			}
		}
	}
	for _, d := range docs {
		if d.Queue == nil || len(d.Queue.Spec.Roles) == 0 {
			continue
		}
		ns := d.Queue.Metadata.Namespace
		for i, r := range d.Queue.Spec.Roles {
			if !roles[ns][r] {
				issues = append(issues, Issue{Severity: SeverityError, Code: "UNKNOWN_ROLE", File: d.File, Resource: d.ID(), Path: fmt.Sprintf("spec.roles[%d]", i),
					Message: fmt.Sprintf("no Team member in namespace %s holds role %q: nobody could answer this queue", ns, r), Fix: "add a Team with a member holding it"})
			}
		}
	}
	return issues
}

func splitAct(ref string) (string, string) {
	for i := 0; i < len(ref); i++ {
		if ref[i] == '/' {
			return ref[:i], ref[i+1:]
		}
	}
	return ref, ""
}

// oneAddress says whether s is exactly one bare e-mail address: no list
// ("a@x, b@y"), no display name, no comment.
func oneAddress(s string) bool {
	a, err := mail.ParseAddress(s)
	return err == nil && a.Name == "" && a.Address == s && strings.Count(s, "@") == 1 && strings.Contains(s[strings.Index(s, "@"):], ".")
}

// reachable says whether some member holding one of roles has an address
// on channel.
func reachable(members []agents.Member, roles []string, channel string) bool {
	for _, m := range members {
		addr := map[string]string{"whatsapp": m.WhatsApp, "slack": m.Slack, "teams": m.Teams, "email": m.Email}[channel]
		if addr == "" {
			continue
		}
		for _, r := range m.Roles {
			for _, q := range roles {
				if r == q {
					return true
				}
			}
		}
	}
	return false
}
