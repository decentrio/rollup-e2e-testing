package relayer

import (
	"github.com/decentrio/rollup-e2e-testing/ibc"
	"github.com/docker/docker/client"
	"go.uber.org/zap"
)

type TestName interface {
	Name() string
}

// RelayerFactory describes how to start a Relayer.
type RelayerFactory interface {
	// Build returns a Relayer associated with the given arguments.
	Build(
		t TestName,
		cli *client.Client,
		networkID string,
	) ibc.Relayer

	// Name returns a descriptive name of the factory,
	// indicating details of the Relayer that will be built.
	Name() string
}

// builtinRelayerFactory is the built-in relayer factory that understands
// how to start the cosmos relayer in a docker container.
type builtinRelayerFactory struct {
	log     *zap.Logger
	options []RelayerOpt
	version string
}

func NewBuiltinRelayerFactory(logger *zap.Logger, options ...RelayerOpt) RelayerFactory {
	return &builtinRelayerFactory{log: logger, options: options}
}

// Build returns a relayer chosen depending on f.impl.
func (f *builtinRelayerFactory) Build(
	t TestName,
	cli *client.Client,
	networkID string,
) ibc.Relayer {
	r := NewCosmosRelayer(
		f.log,
		t.Name(),
		cli,
		networkID,
		f.options...,
	)
	f.setRelayerVersion(r.ContainerImage())
	return r
}

func (f *builtinRelayerFactory) setRelayerVersion(di ibc.DockerImage) {
	f.version = di.Version
}

func (f *builtinRelayerFactory) Name() string {
	if f.version == "" {
		return "rly@" + f.version
	}
	return "rly@" + DefaultContainerVersion
}
