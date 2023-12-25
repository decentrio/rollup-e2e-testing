package testreporter_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/decentrio/rollup-e2e-testing/testreporter"
	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/require"
)

func TestWrappedMessage_RoundTrip(t *testing.T) {
	tcs := []struct {
		Message testreporter.Message
	}{
		{Message: testreporter.BeginSuiteMessage{StartedAt: time.Now()}},
		{Message: testreporter.FinishSuiteMessage{FinishedAt: time.Now()}},
		{
			Message: testreporter.BeginTestMessage{
				Name:      "foo",
				StartedAt: time.Now(),
			},
		},
		{Message: testreporter.PauseTestMessage{Name: "foo", When: time.Now()}},
		{Message: testreporter.ContinueTestMessage{Name: "foo", When: time.Now()}},
		{Message: testreporter.FinishTestMessage{Name: "foo", FinishedAt: time.Now(), Skipped: true, Failed: true}},
		{Message: testreporter.TestErrorMessage{Name: "foo", When: time.Now(), Message: "something failed"}},
		{Message: testreporter.TestSkipMessage{Name: "foo", When: time.Now(), Message: "skipped for reasons"}},
		{
			Message: testreporter.RelayerExecMessage{
				Name:          "foo",
				StartedAt:     time.Now(),
				FinishedAt:    time.Now().Add(time.Second),
				ContainerName: "relayer-exec-123",
				Command:       []string{"rly", "version"},
				Stdout:        "relayer v1.2.3",
				ExitCode:      0,
				Error:         "",
			},
		},
	}

	for _, tc := range tcs {
		wrapped := testreporter.JSONMessage(tc.Message)

		out, err := json.Marshal(wrapped)
		require.NoError(t, err)

		var unwrapped testreporter.WrappedMessage
		require.NoError(t, json.Unmarshal(out, &unwrapped))

		diff := cmp.Diff(wrapped, unwrapped)
		require.Empty(t, diff)
	}
}
