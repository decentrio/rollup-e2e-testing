package dymshub

import (
	"context"
	"fmt"

	sdkmath "cosmossdk.io/math"
	"github.com/decentrio/rollup-e2e-testing/cosmos"
	"github.com/decentrio/rollup-e2e-testing/ibc"
	"go.uber.org/zap"
)

type DymsHub struct {
	*cosmos.CosmosChain
	rollApp ibc.RollApp
}

var _ ibc.Chain = (*DymsHub)(nil)
var _ ibc.RollHub = (*DymsHub)(nil)

const (
	sequencerName = "sequencer"
	maxSequencers = "5"
)

func NewDymsHub(testName string, chainConfig ibc.ChainConfig, numValidators int, numFullNodes int, log *zap.Logger) *DymsHub {
	cosmosChain := cosmos.NewCosmosChain(testName, chainConfig, numValidators, numFullNodes, log)

	c := &DymsHub{
		CosmosChain: cosmosChain,
	}

	return c
}

func (c *DymsHub) Start(testName string, ctx context.Context, seq string, additionalGenesisWallets ...ibc.WalletData) error {
	// Start chain
	err := c.CosmosChain.Start(testName, ctx, seq, additionalGenesisWallets...)
	if err != nil {
		return err
	}
	if err := c.CreateHubKey(ctx, sequencerName); err != nil {
		return err
	}
	sequencer, err := c.AccountHubKeyBech32(ctx, sequencerName)
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

	rollAppChainID := c.rollApp.GetChainID()
	keyDir := c.rollApp.GetKeyDir()
	if err := c.RegisterRollAppToHub(ctx, sequencerName, rollAppChainID, maxSequencers, keyDir); err != nil {
		return fmt.Errorf("failed to start chain %s: %w", c.Config().Name, err)
	}
	if err := c.RegisterSequencerToHub(ctx, sequencerName, rollAppChainID, maxSequencers, seq, keyDir); err != nil {
		return fmt.Errorf("failed to start chain %s: %w", c.Config().Name, err)
	}
	return nil
}

// Implements Chain interface
func (c *DymsHub) CreateHubKey(ctx context.Context, keyName string) error {
	return c.GetNode().CreateHubKey(ctx, keyName)
}

// RegisterSequencerToHub register sequencer for rollapp on settlement.
func (c *DymsHub) RegisterSequencerToHub(ctx context.Context, keyName, rollappChainID, maxSequencers, seq, keyDir string) error {
	return c.GetNode().RegisterSequencerToHub(ctx, keyName, rollappChainID, maxSequencers, seq, keyDir)
}

// RegisterRollAppToHub register rollapp on settlement.
func (c *DymsHub) RegisterRollAppToHub(ctx context.Context, keyName, rollappChainID, maxSequencers, keyDir string) error {
	return c.GetNode().RegisterRollAppToHub(ctx, keyName, rollappChainID, maxSequencers, keyDir)
}

func (c *DymsHub) SetRollApp(rollApp ibc.RollApp) {
	c.rollApp = rollApp
}
