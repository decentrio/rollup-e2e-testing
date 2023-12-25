package rollupe2etesting

import (
	"github.com/decentrio/rollup-e2e-testing/dockerutil"
	"github.com/docker/docker/client"
)

const (
	FaucetAccountKeyName = "faucet"
)

// KeepDockerVolumesOnFailure sets whether volumes associated with a particular test
// are retained or deleted following a test failure.
//
// The value is false by default, but can be initialized to true by setting the
// environment variable IBCTEST_SKIP_FAILURE_CLEANUP to a non-empty value.
// Alternatively, importers of the interchaintest package may call KeepDockerVolumesOnFailure(true).
func KeepDockerVolumesOnFailure(b bool) {
	dockerutil.KeepVolumesOnFailure = b
}

// DockerSetup returns a new Docker Client and the ID of a configured network, associated with t.
//
// If any part of the setup fails, t.Fatal is called.
func DockerSetup(t dockerutil.DockerSetupTestingT) (*client.Client, string) {
	t.Helper()
	return dockerutil.DockerSetup(t)
}
