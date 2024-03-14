package gm_rollapp

import (
	"github.com/decentrio/rollup-e2e-testing/cosmos"
	"github.com/decentrio/rollup-e2e-testing/ibc"
	"go.uber.org/zap"
)

type GmRollApp struct {
	*cosmos.CosmosChain
}

func NewGmRollApp(testName string, chainConfig ibc.ChainConfig, numValidators int, numFullNodes int, log *zap.Logger) *GmRollApp {
	cosmosChain := cosmos.NewCosmosChain(testName, chainConfig, numValidators, numFullNodes, log)

	c := &GmRollApp{
		CosmosChain: cosmosChain,
	}

	return c
}
