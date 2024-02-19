package dyms

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"os"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/decentrio/rollup-e2e-testing/cosmos"
	"github.com/decentrio/rollup-e2e-testing/ibc"
	"github.com/decentrio/rollup-e2e-testing/testutil"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
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

	return &DymsChain{
		CosmosChain: cosmosChain,
	}
}

func (c *DymsChain) Start(testName string, ctx context.Context, seq string, additionalGenesisWallets ...ibc.WalletData) error {
	nodes := c.Nodes()

	if err := nodes.LogGenesisHashes(ctx); err != nil {
		return err
	}

	eg, egCtx := errgroup.WithContext(ctx)
	for _, n := range nodes {
		n := n
		eg.Go(func() error {
			return n.CreateNodeContainer(egCtx)
		})
	}
	if err := eg.Wait(); err != nil {
		return err
	}

	peers := nodes.PeerString(ctx)

	eg, egCtx = errgroup.WithContext(ctx)
	for _, n := range nodes {
		n := n
		c.Logger().Info("Starting container", zap.String("container", n.Name()))
		eg.Go(func() error {
			if err := n.SetPeers(egCtx, peers); err != nil {
				return err
			}
			return n.StartContainer(egCtx)
		})
	}
	if err := eg.Wait(); err != nil {
		return err
	}

	// Wait for 5 blocks before considering the chains "started"
	return testutil.WaitForBlocks(ctx, 5, c.GetNode())
}

func (c *DymsChain) Configuration(testName string, ctx context.Context, additionalGenesisWallets ...ibc.WalletData) (string, error) {
	chainCfg := c.Config()

	decimalPow := int64(math.Pow10(int(*chainCfg.CoinDecimals)))

	genesisAmount := sdk.Coin{
		Amount: sdkmath.NewInt(100_000_000_000_000).MulRaw(decimalPow),
		Denom:  chainCfg.Denom,
	}

	genesisSelfDelegation := sdk.Coin{
		Amount: sdkmath.NewInt(50_000_000_000_000).MulRaw(decimalPow),
		Denom:  chainCfg.Denom,
	}

	if chainCfg.ModifyGenesisAmounts != nil {
		genesisAmount, genesisSelfDelegation = chainCfg.ModifyGenesisAmounts()
	}

	genesisAmounts := []sdk.Coin{genesisAmount}

	configFileOverrides := chainCfg.ConfigFileOverrides

	eg := new(errgroup.Group)
	// Initialize config and sign gentx for each validator.
	for _, v := range c.Validators {
		v := v
		keyDir = v.HomeDir()
		v.Validator = true
		eg.Go(func() error {
			if err := v.InitFullNodeFiles(ctx); err != nil {
				return err
			}
			for configFile, modifiedConfig := range configFileOverrides {
				modifiedToml, ok := modifiedConfig.(testutil.Toml)
				if !ok {
					return fmt.Errorf("Provided toml override for file %s is of type (%T). Expected (DecodedToml)", configFile, modifiedConfig)
				}
				if err := testutil.ModifyTomlConfigFile(
					ctx,
					v.Logger(),
					v.DockerClient,
					v.TestName,
					v.VolumeName,
					v.Chain.Config().Name,
					configFile,
					modifiedToml,
				); err != nil {
					return err
				}
			}
			if !chainCfg.SkipGenTx {
				return v.InitValidatorGenTx(ctx, &chainCfg, genesisAmounts, genesisSelfDelegation)
			}
			return nil
		})
	}

	// Initialize config for each full node.
	for _, n := range c.FullNodes {
		n := n
		n.Validator = false
		eg.Go(func() error {
			if err := n.InitFullNodeFiles(ctx); err != nil {
				return err
			}
			for configFile, modifiedConfig := range configFileOverrides {
				modifiedToml, ok := modifiedConfig.(testutil.Toml)
				if !ok {
					return fmt.Errorf("Provided toml override for file %s is of type (%T). Expected (DecodedToml)", configFile, modifiedConfig)
				}
				if err := testutil.ModifyTomlConfigFile(
					ctx,
					n.Logger(),
					n.DockerClient,
					n.TestName,
					n.VolumeName,
					n.Chain.Config().Name,
					configFile,
					modifiedToml,
				); err != nil {
					return err
				}
			}
			return nil
		})
	}

	// wait for this to finish
	if err := eg.Wait(); err != nil {
		return "", err
	}

	if chainCfg.PreGenesis != nil {
		err := chainCfg.PreGenesis(chainCfg)
		if err != nil {
			return "", err
		}
	}

	// for the validators we need to collect the gentxs and the accounts
	// to the first node's genesis file
	validator0 := c.Validators[0]
	for i := 1; i < len(c.Validators); i++ {
		validatorN := c.Validators[i]

		bech32, err := validatorN.AccountKeyBech32(ctx, valKey)
		if err != nil {
			return "", err
		}

		if err := validator0.AddGenesisAccount(ctx, bech32, genesisAmounts); err != nil {
			return "", err
		}

		if !chainCfg.SkipGenTx {
			if err := validatorN.CopyGentx(ctx, validator0); err != nil {
				return "", err
			}
		}
	}
	for _, wallet := range additionalGenesisWallets {

		if err := validator0.AddGenesisAccount(ctx, wallet.Address, []sdk.Coin{{Denom: wallet.Denom, Amount: wallet.Amount}}); err != nil {
			return "", err
		}
	}

	if !chainCfg.SkipGenTx {
		if err := validator0.CollectGentxs(ctx); err != nil {
			return "", err
		}
	}

	genbz, err := validator0.GenesisFileContent(ctx)
	if err != nil {
		return "", err
	}

	genbz = bytes.ReplaceAll(genbz, []byte(`"stake"`), []byte(fmt.Sprintf(`"%s"`, chainCfg.Denom)))

	if chainCfg.ModifyGenesis != nil {
		genbz, err = chainCfg.ModifyGenesis(chainCfg, genbz)
		if err != nil {
			return "", err
		}
	}

	// Provide EXPORT_GENESIS_FILE_PATH and EXPORT_GENESIS_CHAIN to help debug genesis file
	exportGenesis := os.Getenv("EXPORT_GENESIS_FILE_PATH")
	exportGenesisChain := os.Getenv("EXPORT_GENESIS_CHAIN")
	if exportGenesis != "" && exportGenesisChain == chainCfg.Name {
		c.Logger().Debug("Exporting genesis file",
			zap.String("chain", exportGenesisChain),
			zap.String("path", exportGenesis),
		)
		_ = os.WriteFile(exportGenesis, genbz, 0600)
	}
	nodes := c.Nodes()

	for _, node := range nodes {
		if err := node.OverwriteGenesisFile(ctx, genbz); err != nil {
			return "", err
		}
	}
	seq, err := c.ShowSeq(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to show seq %s: %w", c.Config().Name, err)
	}
	return seq, nil
}
