package celes_hub

import (
	"context"

	"github.com/decentrio/rollup-e2e-testing/cosmos"
	"github.com/decentrio/rollup-e2e-testing/ibc"
	"go.uber.org/zap"
)

type CelesHub struct {
	*cosmos.CosmosChain
}

var _ ibc.Chain = (*CelesHub)(nil)

// var _ ibc.Hub = (*CelesHub)(nil)

func NewCelesHub(testName string, chainConfig ibc.ChainConfig, numValidators int, numFullNodes int, log *zap.Logger) *CelesHub {
	cosmosChain := cosmos.NewCosmosChain(testName, chainConfig, numValidators, numFullNodes, log)

	c := &CelesHub{
		CosmosChain: cosmosChain,
	}

	return c
}

func (c *CelesHub) Start(testName string, ctx context.Context, additionalGenesisWallets ...ibc.WalletData) error {
	// Start chain
	err := c.CosmosChain.Start(testName, ctx, additionalGenesisWallets...)
	if err != nil {
		return err
	}
	return nil
}
