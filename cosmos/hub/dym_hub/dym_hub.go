package dym_hub

import (
	"context"
	"fmt"

	sdkmath "cosmossdk.io/math"
	"github.com/decentrio/rollup-e2e-testing/cosmos"
	"github.com/decentrio/rollup-e2e-testing/ibc"
	"go.uber.org/zap"
)

type DymHub struct {
	*cosmos.CosmosChain
	rollApp ibc.RollApp
}

var _ ibc.Chain = (*DymHub)(nil)
var _ ibc.Hub = (*DymHub)(nil)

const (
	sequencerName = "sequencer"
	maxSequencers = "5"
)

func NewDymHub(testName string, chainConfig ibc.ChainConfig, numValidators int, numFullNodes int, log *zap.Logger) *DymHub {
	cosmosChain := cosmos.NewCosmosChain(testName, chainConfig, numValidators, numFullNodes, log)

	c := &DymHub{
		CosmosChain: cosmosChain,
	}

	return c
}

func (c *DymHub) Start(testName string, ctx context.Context, additionalGenesisWallets ...ibc.WalletData) error {
	// Start chain
	err := c.CosmosChain.Start(testName, ctx, additionalGenesisWallets...)
	if err != nil {
		return err
	}

	// if not have rollApp, we just return the function
	if c.rollApp == nil {
		return nil
	}

	rollAppChainID := c.GetRollApp().(ibc.Chain).GetChainID()
	keyDir := c.GetRollApp().GetSequencerKeyDir()
	seq := c.GetRollApp().GetSequencer()

	if err := c.GetNode().CreateKeyWithKeyDir(ctx, sequencerName, keyDir); err != nil {
		return err
	}
	sequencer, err := c.AccountKeyBech32WithKeyDir(ctx, sequencerName, keyDir)
	if err != nil {
		return err
	}
	amount := sdkmath.NewInt(10_000_000_000_000)
	fund := ibc.WalletData{
		Address: sequencer,
		Denom:   c.Config().Denom,
		Amount:  amount,
	}
	if err := c.SendFunds(ctx, "faucet", fund); err != nil {
		return err
	}

	if err := c.RegisterRollAppToHub(ctx, sequencerName, rollAppChainID, maxSequencers, keyDir); err != nil {
		return fmt.Errorf("failed to start chain %s: %w", c.Config().Name, err)
	}
	if err := c.RegisterSequencerToHub(ctx, sequencerName, rollAppChainID, maxSequencers, seq, keyDir); err != nil {
		return fmt.Errorf("failed to start chain %s: %w", c.Config().Name, err)
	}
	return nil
}

// RegisterSequencerToHub register sequencer for rollapp on settlement.
func (c *DymHub) RegisterSequencerToHub(ctx context.Context, keyName, rollappChainID, maxSequencers, seq, keyDir string) error {
	return c.GetNode().RegisterSequencerToHub(ctx, keyName, rollappChainID, maxSequencers, seq, keyDir)
}

// RegisterRollAppToHub register rollapp on settlement.
func (c *DymHub) RegisterRollAppToHub(ctx context.Context, keyName, rollappChainID, maxSequencers, keyDir string) error {
	return c.GetNode().RegisterRollAppToHub(ctx, keyName, rollappChainID, maxSequencers, keyDir)
}

func (c *DymHub) SetRollApp(rollApp ibc.RollApp) {
	c.rollApp = rollApp
}

func (c *DymHub) GetRollApp() ibc.RollApp {
	return c.rollApp
}

func (c *DymHub) FullfillDemandOrder(ctx context.Context,
	node *cosmos.Node,
	id string,
	keyName string,
) (txhash string, _ error) {
	command := []string{
		"tx", "eibc",
		"fulfill-order", id,
	}
	return node.ExecTx(ctx, keyName, command...)
}
