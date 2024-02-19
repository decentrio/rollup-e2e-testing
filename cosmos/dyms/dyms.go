package dyms

import (
	"context"

	"github.com/decentrio/rollup-e2e-testing/cosmos"
	"github.com/decentrio/rollup-e2e-testing/ibc"
	"go.uber.org/zap"
)

const (
	valKey = "validator"
)

var keyDir string

type DymsChain struct {
	*cosmos.CosmosChain
}

var _ ibc.Chain = (*DymsChain)(nil)
var _ ibc.RollAppChain = (*DymsChain)(nil)

func NewDymsChain(testName string, chainConfig ibc.ChainConfig, numValidators int, numFullNodes int, log *zap.Logger) *DymsChain {
	cosmosChain := cosmos.NewCosmosChain(testName, chainConfig, numValidators, numFullNodes, log)

	c := &DymsChain{
		CosmosChain: cosmosChain,
	}

	return c
}

// func (c *DymsChain) Start(testName string, ctx context.Context, seq string, additionalGenesisWallets ...ibc.WalletData) error {
// 	nodes := c.Nodes()

// 	if err := nodes.LogGenesisHashes(ctx); err != nil {
// 		return err
// 	}

// 	eg, egCtx := errgroup.WithContext(ctx)
// 	for _, n := range nodes {
// 		n := n
// 		eg.Go(func() error {
// 			return n.CreateNodeContainer(egCtx)
// 		})
// 	}
// 	if err := eg.Wait(); err != nil {
// 		return err
// 	}

// 	peers := nodes.PeerString(ctx)

// 	eg, egCtx = errgroup.WithContext(ctx)
// 	for _, n := range nodes {
// 		n := n
// 		c.Logger().Info("Starting container", zap.String("container", n.Name()))
// 		eg.Go(func() error {
// 			if err := n.SetPeers(egCtx, peers); err != nil {
// 				return err
// 			}
// 			return n.StartContainer(egCtx)
// 		})
// 	}
// 	if err := eg.Wait(); err != nil {
// 		return err
// 	}

// 	// Wait for 5 blocks before considering the chains "started"
// 	return testutil.WaitForBlocks(ctx, 5, c.GetNode())
// }

func (c *DymsChain) Configuration(testName string, ctx context.Context, additionalGenesisWallets ...ibc.WalletData) (string, error) {
	return c.CreateRollapp(testName, ctx, additionalGenesisWallets...)
}
