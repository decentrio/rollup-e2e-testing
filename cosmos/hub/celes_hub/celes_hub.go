package celes_hub

import (
	"context"
	"fmt"

	"github.com/decentrio/rollup-e2e-testing/cosmos"
	"github.com/decentrio/rollup-e2e-testing/ibc"
	util "github.com/decentrio/rollup-e2e-testing/testutil"
	"go.uber.org/zap"
)

type CelesHub struct {
	*cosmos.CosmosChain
}

var _ ibc.Chain = (*CelesHub)(nil)

var _ ibc.Hub = (*CelesHub)(nil)

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
	if err := c.RegisterEVMValidatorToHub(ctx, "validator"); err != nil {
		return fmt.Errorf("failed to start chain %s: %w", c.Config().Name, err)
	}
	//cp -r $APP_PATH/keyring-test/ $NODE_PATH/keys/keyring-test/
	src := "/tmp/" + c.HomeDir() + "/keyring-test/"
	dst := "/tmp/celestia/bridge/keys/keyring-test/"
	util.CopyDir(src, dst)

	return nil
}

// RegisterEVMValidatorToHub register the validator EVM address.
func (c *CelesHub) RegisterEVMValidatorToHub(ctx context.Context, keyName string) error {
	return c.GetNode().RegisterEVMValidatorToHub(ctx, keyName)
}

// RegisterSequencerToHub register sequencer for rollapp on settlement.
func (c *CelesHub) RegisterSequencerToHub(ctx context.Context, keyName, rollappChainID, maxSequencers, seq, keyDir string) error {
	return c.GetNode().RegisterSequencerToHub(ctx, keyName, rollappChainID, maxSequencers, seq, keyDir)
}

// RegisterRollAppToHub register rollapp on settlement.
func (c *CelesHub) RegisterRollAppToHub(ctx context.Context, keyName, rollappChainID, maxSequencers, keyDir string) error {
	return c.GetNode().RegisterRollAppToHub(ctx, keyName, rollappChainID, maxSequencers, keyDir)
}

func (c *CelesHub) SetRollApp(rollApp ibc.RollApp) {
	// Todo
}

func (c *CelesHub) GetRollApp() ibc.RollApp {
	// Todo
	return nil
}
