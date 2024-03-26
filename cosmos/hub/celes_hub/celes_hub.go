package celes_hub

import (
	"context"
	"fmt"
	"strings"

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
	// DA bridge parameters
	var (
		nodeStore = "/home/celestia/bridge"
		//coreIp      = "127.0.0.1"
		coreIp      = fmt.Sprintf("tcp://%s", c.GetNode().Name())
		accName     = "validator"
		gatewayAddr = "0.0.0.0"
		rpcAddr     = "0.0.0.0"
		heightQuery = "1"
	)

	// Start chain
	err := c.CosmosChain.Start(testName, ctx, additionalGenesisWallets...)
	if err != nil {
		return err
	}
	if err := c.RegisterEVMValidatorToHub(ctx, "validator"); err != nil {
		return fmt.Errorf("failed to start chain %s: %w", c.Config().Name, err)
	}
	// copy data from app path to node path
	tmp := strings.Split(c.HomeDir(), "/")
	src := "/tmp/" + tmp[len(tmp)-1] + "/keyring-test"
	dst := "/tmp/celestia/bridge/keys/keyring-test"
	util.CopyDir(src, dst)

	hash, err := c.GetNode().GetHashOfBlockHeight(ctx, heightQuery)
	if err != nil {
		return fmt.Errorf("failed to fetch hash of block height %s: %w", heightQuery, err)
	}
	env := []string{"CELESTIA_CUSTOM=test:" + hash}

	// initialize bridge
	err = c.GetNode().CelestiaDaBridgeInit(ctx, nodeStore, env)
	if err != nil {
		return err
	}

	// start bridge
	err = c.GetNode().CelestiaDaBridgeStart(ctx, nodeStore, coreIp, accName, gatewayAddr, rpcAddr, env)
	if err != nil {
		return err
	}

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
