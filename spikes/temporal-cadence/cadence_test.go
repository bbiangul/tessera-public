package cadence

import (
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"go.temporal.io/sdk/testsuite"
)

func TestCadence(t *testing.T) {
	var s testsuite.WorkflowTestSuite
	env := s.NewTestWorkflowEnvironment()
	start := time.Date(2026, 9, 25, 15, 0, 0, 0, time.UTC) // Friday 12h in São Paulo
	env.SetStartTime(start)
	var sent []string
	var at []time.Time
	env.RegisterActivity(Send)
	env.OnActivity(Send, mock.Anything).Return(func(text string) error {
		sent = append(sent, text)
		at = append(at, env.Now())
		return nil
	})
	env.ExecuteWorkflow(Cadence, "lead-1")
	if !env.IsWorkflowCompleted() || env.GetWorkflowError() != nil {
		t.Fatalf("completed %v, error %v", env.IsWorkflowCompleted(), env.GetWorkflowError())
	}
	monday10 := time.Date(2026, 9, 28, 10, 0, 0, 0, saoPaulo)
	if len(sent) != 3 || !at[1].Equal(monday10) || !at[2].Equal(monday10.Add(72*time.Hour)) {
		t.Errorf("sent %q at %v", sent, at)
	}
}
