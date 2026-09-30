package host

import "github.com/bbiangul/tessera/internal/engines/decide"

const preferenceContext = " sideMessages contains earlier user messages already answered while waiting for this reply. Use their scheduling preferences to interpret a continuation such as permission to search those options; that continuation is answer. Do not answer their old factual questions again; classify only the current message."
const preferenceInformation = "Does the message ask for factual information independent of the scheduling preference, such as parking, address, insurance, price or contact details? Answer true even when the message also supplies a preferred day/time, and even when the question is not a prerequisite. Asking for a different date/time or permission to search options is false. A cancellation or goodbye alone is false." + " Read ONLY the current STATE.message to answer this question. Ignore all questions in STATE.sideMessages: those older questions have already been answered. Their presence does not make the current message an information question."
const preferenceStop = "Using ONLY the current STATE.message, does the user want to stop or give up THIS pending rescheduling process, keeping their existing appointment unchanged? Explicit stop/leave it as it is/no longer want to change means true. Requesting cancellation of an appointment, especially another person or another appointment, is a different service and means false. A date correction, read-only search continuation, or factual question alone is false. Ignore earlier sideMessages for this decision."

func preferenceQuestions(req *decide.Request) {
	q := req.Questions["turn"]
	q.Instructions += preferenceContext
	req.Questions["turn"] = q
	req.Questions["information"] = decide.Question{Kind: decide.KindNoul, Instructions: preferenceInformation, IfTrue: "An independent factual question is present.", IfFalse: "No independent factual question."}
	req.Questions["stopCurrent"] = decide.Question{Kind: decide.KindNoul, Instructions: preferenceStop, IfTrue: "Stop this rescheduling process without changing the existing appointment.", IfFalse: "Not a withdrawal from this process."}
}
