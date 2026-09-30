package v1alpha1

// KnowledgeBase is a set of the business's documents the agent answers
// from (RFC-0008 part A, knowledge-001): its policies, FAQ, price lists.
// Each passage keeps its source; its text is data, never instructions.
type KnowledgeBase struct {
	// APIVersion must be agents.tessera.dev/v1alpha1.
	APIVersion string `json:"apiVersion" jsonschema:"enum=agents.tessera.dev/v1alpha1"`
	// Kind must be KnowledgeBase.
	Kind string `json:"kind" jsonschema:"enum=KnowledgeBase"`
	// Metadata names the base.
	Metadata ObjectMeta `json:"metadata"`
	// Spec says where its documents are and who may read them.
	Spec KnowledgeBaseSpec `json:"spec"`
}

// KnowledgeBaseSpec is a knowledge base's sources and audiences.
type KnowledgeBaseSpec struct {
	// Description says what the documents are about.
	Description string `json:"description" jsonschema:"minLength=1"`
	// Sources are where the documents come from. A base with lessons may
	// have none.
	Sources []KnowledgeSource `json:"sources,omitempty"`
	// Lessons make this base learn from the team's answers (learn-003):
	// every so often the agent proposes lessons, a person approves each
	// one in the team's channel, and approved lessons join this base.
	Lessons *KnowledgeLessons `json:"lessons,omitempty"`
	// Audiences are the Agent's audiences whose callers may receive its
	// passages (RFC-0006). Empty: every caller. A staff-only policy names
	// the team's audience.
	Audiences []string `json:"audiences,omitempty"`
}

// KnowledgeSource is one place documents come from: files, or a plugin
// (list, and fetch for each listed document). Exactly one of files and
// list.
type KnowledgeSource struct {
	// Files are glob patterns, relative to this resource's file, of
	// Markdown (.md), text (.txt), PDF or DOCX documents. Each is split by
	// headings and paragraphs when the agent loads.
	Files []string `json:"files,omitempty"`
	// List is a read action returning the documents: a list of
	// {name, id?, modified?, text?, base64?}. The name's extension says
	// the type (.md, .txt, .pdf, .docx); other types are skipped
	// (plugin-001).
	List *KnowledgeAct `json:"list,omitempty"`
	// Fetch is a read action called for each listed document without
	// text or base64, with the listed document as inputs ("${ inputs.id }"); it returns
	// {base64} or {text}. A document whose modified did not change is not
	// fetched again.
	Fetch *KnowledgeAct `json:"fetch,omitempty"`
	// Refresh is how often a plugin source is read again (a duration, at
	// least 1m); default 24h. A failed read keeps the previous documents
	// and is retried within 5 minutes.
	Refresh string `json:"refresh,omitempty"`
}

// KnowledgeAct is a plugin action with its arguments.
type KnowledgeAct struct {
	// Act is the read action, plugin/action.
	Act string `json:"act" jsonschema:"pattern=^[a-z0-9-]+/[a-z0-9-]+$"`
	// With gives its arguments. Values are literal unless the whole value
	// is "${ expr }"; fetch reads item.
	With map[string]any `json:"with,omitempty"`
}

// FreeKnowledge gives the free mode the business's documents.
type FreeKnowledge struct {
	// Bases are KnowledgeBase names of this namespace. Before a free
	// answer the masked message is searched in them (full text, BM25) and
	// the best passages the caller may read reach the model as
	// FACTS.documents; the passages given are journaled with the turn.
	Bases []string `json:"bases" jsonschema:"minItems=1"`
	// Check asks Jev, after the answer is written, whether it states a
	// fact that neither the documents nor the other facts support; if so
	// it is rewritten once, then the agent says it does not know. On
	// unless set to false (knowledge-001: unsupported claims 1/90 -> 0/90,
	// about 0.4 s more per answer).
	Check *bool `json:"check,omitempty"`
	// Relevance asks Jev, in one request, which of the passages found
	// answer or directly settle part of the message; the others are
	// dropped before the handoff and the answer (knowledge-002), so a
	// passage on a nearby topic is not taken for an answer. On unless set
	// to false (knowledge-002: misleading replies 14 -> 7 of 42 on new
	// data, handoffs 19 -> 27, answerable unchanged).
	Relevance *bool `json:"relevance,omitempty"`
}

// KnowledgeLessons configures a base that learns (learn-003).
type KnowledgeLessons struct {
	// Queue is the HumanQueue whose members approve lessons; default the
	// Agent's humanInTheLoop.
	Queue string `json:"queue,omitempty"`
	// Every is how often lessons are proposed (a duration, at least 1h);
	// default 168h.
	Every string `json:"every,omitempty"`
	// Min is how many answered cases a lesson needs; default 2.
	Min int `json:"min,omitempty" jsonschema:"minimum=0"`
}
