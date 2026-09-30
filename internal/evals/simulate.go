package evals

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	agents "github.com/bbiangul/tessera/api/agents/v1alpha1"
	"github.com/bbiangul/tessera/internal/engines/generate"
	"github.com/bbiangul/tessera/internal/host"
	"github.com/bbiangul/tessera/internal/ledger"
	"github.com/bbiangul/tessera/internal/runtime/journal"
)

// The llm simulator (RFC-0002, level 4): a model plays the persona, one
// message at a time, reading only what the agent told the user. The team's
// answers come from the scenario's humans, in order. Its cost is metered
// apart from the agent's.

const defaultMaxTurns = 12

const simulatorRules = `You play a customer (the USER) chatting with a company's agent. PERSONA.goal is what you want, PERSONA.style is how you write, PERSONA.knows holds the only facts you have. Write the user's NEXT message.
Rules:
- Pursue the goal like a real person. Start naturally: you do not have to give every detail at once. Answer what the agent asks, from PERSONA.knows.
- Never invent a fact that is not in PERSONA.knows. Asked something you do not know, say so, or pick the simplest answer that fits the goal.
- React to what the agent actually said in CONVERSATION. Given options, choose by the goal. Do not repeat a message you already sent.
- Write only the message: never the agent's part, no stage directions, no explanations.
- done is true, with message null, when the goal is met, when the agent clearly cannot help any further, or when it asked you the same thing three times. A short thanks after the goal is met is fine before that.
- Write in PERSONA.language, in PERSONA.style.`

// simulate plays the persona until it is done, maxTurns is reached, or a
// run waits for a person the scenario has no answer for.
func (r *scenarioRun) simulate(ctx context.Context, sim generate.Generator, h *host.Host, store journal.Store, conv *string, now *time.Time, sc agents.Scenario) error {
	if sim == nil {
		return errors.New("the llm simulator needs a Generate backend (--generate)")
	}
	p := sc.Persona
	lang := p.Language
	if lang == "" {
		lang = "Brazilian Portuguese"
	}
	persona := map[string]any{"goal": p.Goal, "style": p.Style, "knows": p.Knows, "language": lang}
	maxTurns := sc.MaxTurns
	if maxTurns == 0 {
		maxTurns = defaultMaxTurns
	}
	humans := sc.Humans
	schema := map[string]any{"type": "object", "additionalProperties": false, "required": []any{"message", "done"},
		"properties": map[string]any{"message": map[string]any{"type": []any{"string", "null"}}, "done": map[string]any{"type": "boolean"}}}
	turn := len(sc.Turns)
	for n := 0; n < maxTurns; n++ {
		resp, err := sim.Generate(ctx, generate.Request{
			Instructions: simulatorRules,
			State:        map[string]any{"persona": persona, "conversation": r.userView(), "message": n + 1, "maxMessages": maxTurns},
			Schema:       schema,
			MaxTokens:    400,
		})
		if err != nil {
			return fmt.Errorf("simulator: %w", err)
		}
		m, _ := resp.Value.(map[string]any)
		msg, _ := m["message"].(string)
		if done, _ := m["done"].(bool); done || strings.TrimSpace(msg) == "" {
			r.res.Transcript = append(r.res.Transcript, Line{Who: "simulator", Text: "done"})
			return nil
		}
		// A person takes a while to write: the clock moves a minute per
		// message, so two searches of one conversation are two events.
		*now = now.Add(time.Minute)
		if err := r.turn(ctx, h, store, conv, now, sc.Caller, turn, agents.Turn{User: msg}); err != nil {
			return err
		}
		turn++
		// The team answers every task the agent handed to a person.
		for {
			run, ok, err := store.ActiveRun(ctx, *conv)
			if err != nil {
				return err
			}
			if !ok || run.Status != journal.StatusWaitingForHuman {
				break
			}
			task, err := h.Runtime.PendingTask(ctx, run.ID)
			if err != nil {
				return err
			}
			i := fitting(humans, task.Kind)
			if i < 0 && r.teamFacts != "" {
				// The simulated team answers from its facts (team-001).
				answer, err := teamAnswer(ctx, sim, r.teamFacts, task, r.userView())
				if err != nil {
					return fmt.Errorf("team: %w", err)
				}
				r.res.TeamAnswers++
				if err := r.turn(ctx, h, store, conv, now, sc.Caller, turn, agents.Turn{Human: answer}); err != nil {
					return err
				}
				turn++
				continue
			}
			if i < 0 {
				// The queue and kind tell a booking waiting for the team from
				// a question handed to it (agency-005's report mixed them).
				r.res.Transcript = append(r.res.Transcript, Line{Who: "simulator", Text: fmt.Sprintf("a person's task is open and the scenario has no answer for it (queue %s, %s)", task.Queue, task.Kind)})
				return nil
			}
			if err := r.turn(ctx, h, store, conv, now, sc.Caller, turn, agents.Turn{Human: humans[i]}); err != nil {
				return err
			}
			humans = append(humans[:i:i], humans[i+1:]...)
			turn++
		}
	}
	r.res.Transcript = append(r.res.Transcript, Line{Who: "simulator", Text: fmt.Sprintf("stopped after %d messages", maxTurns)})
	return nil
}

// userView is the conversation as the user saw it: their messages and the
// agent's, without the team's queues or the clock.
func (r *scenarioRun) userView() []any {
	var out []any
	for _, l := range r.res.Transcript {
		switch l.Who {
		case "user":
			out = append(out, map[string]any{"who": "user", "text": l.Text})
		case "agent":
			out = append(out, map[string]any{"who": "agent", "text": l.Text})
		}
	}
	return out
}

// simulatorLedger meters the simulator apart from the agent, so a chatty
// simulated user does not hide an efficient agent (RFC-0002).
func simulatorLedger(max float64) *ledger.Ledger {
	return ledger.New(ledger.Budget{MaxCostUSD: max})
}

// fitting is the first of the scenario's human answers a task of kind
// takes, or -1: an answer task takes text, an input task a value that is
// not text (agency-004: a booking's reply given to a question broke the
// question's run). Other kinds take the next answer.
func fitting(humans []any, kind string) int {
	for i, x := range humans {
		_, text := x.(string)
		switch {
		case kind == "answer" && !text, kind == "input" && text:
			continue
		}
		return i
	}
	return -1
}
