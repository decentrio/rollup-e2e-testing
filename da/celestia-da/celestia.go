package celestia_da

import (
	"context"

	"github.com/decentrio/rollup-e2e-testing/cosmos"
	"github.com/decentrio/rollup-e2e-testing/da"
	"github.com/docker/docker/client"
	"go.uber.org/zap"
)

type CelestiaDANode struct {
	*da.DockerDANode
}

func NewCelestiaDANode(log *zap.Logger, testName string, cli *client.Client, networkID string, celestia *cosmos.CosmosChain) *CelestiaDANode {
	c := commander{log: log}

	daNode, err := da.NewDockerDANode(context.TODO(), log, testName, cli, networkID, c, celestia)
	if err != nil {
		panic(err) // TODO: return
	}

	celestiaDANode := &CelestiaDANode{
		DockerDANode: daNode,
	}

	return celestiaDANode
}

type commander struct {
	log *zap.Logger
}

func (commander) Init(homeDir string) []string {
	return []string{
		"celestia-da", "bridge", "init", "--node.store", homeDir,
	}
}

func (commander) Start() []string {
	// TODO: da command
	return []string{
		"celestia-da", "bridge", "start",
	}
}
