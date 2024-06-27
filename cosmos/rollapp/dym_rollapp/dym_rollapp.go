package dym_rollapp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/decentrio/rollup-e2e-testing/cosmos"
	"github.com/decentrio/rollup-e2e-testing/dymension"
	"github.com/decentrio/rollup-e2e-testing/ibc"
	"github.com/decentrio/rollup-e2e-testing/testutil"
	"github.com/icza/dyno"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
)

const (
	valKey = "validator"
)

type DymRollApp struct {
	*cosmos.CosmosChain
	sequencerKeyDir string
	sequencerKey    string
	extraFlags      map[string]interface{}
}

var _ ibc.Chain = (*DymRollApp)(nil)
var _ ibc.RollApp = (*DymRollApp)(nil)

func NewDymRollApp(testName string, chainConfig ibc.ChainConfig, numValidators int, numFullNodes int, log *zap.Logger, extraFlags map[string]interface{}) *DymRollApp {
	cosmosChain := cosmos.NewCosmosChain(testName, chainConfig, numValidators, numFullNodes, log)

	c := &DymRollApp{
		CosmosChain: cosmosChain,
		extraFlags:  extraFlags,
	}

	return c
}

func (c *DymRollApp) Start(testName string, ctx context.Context, additionalGenesisWallets ...ibc.WalletData) error {
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
	return nil
}

func (c *DymRollApp) Configuration(testName string, ctx context.Context, forkRollAppId string, gensisContent []byte, additionalGenesisWallets ...ibc.WalletData) error {
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
	for i, v := range c.Validators {
		i := i
		v := v
		c.sequencerKeyDir = v.HomeDir()
		v.Chain = c
		v.Validator = true
		eg.Go(func() error {
			if err := v.InitFullNodeFiles(ctx); err != nil {
				return err
			}
			for configFile, modifiedConfig := range configFileOverrides {
				modifiedToml, ok := modifiedConfig.(testutil.Toml)
				if !ok {
					return fmt.Errorf("provided toml override for file %s is of type (%T). Expected (DecodedToml)", configFile, modifiedConfig)
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
			if !c.Config().SkipGenTx {
				return c.InitValidatorGenTx(ctx, v, i, &chainCfg, genesisAmounts, genesisSelfDelegation)
			}
			return nil
		})
	}

	// Initialize config for each full node.
	for _, n := range c.FullNodes {
		n := n
		n.Validator = false
		n.Chain = c
		eg.Go(func() error {
			if err := n.InitFullNodeFiles(ctx); err != nil {
				return err
			}
			for configFile, modifiedConfig := range configFileOverrides {
				modifiedToml, ok := modifiedConfig.(testutil.Toml)
				if !ok {
					return fmt.Errorf("provided toml override for file %s is of type (%T). Expected (DecodedToml)", configFile, modifiedConfig)
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
		return err
	}

	if c.Config().PreGenesis != nil {
		err := c.Config().PreGenesis(chainCfg)
		if err != nil {
			return err
		}
	}

	// for the validators we need to collect the gentxs and the accounts
	// to the first node's genesis file
	validator0 := c.Validators[0]
	for i := 1; i < len(c.Validators); i++ {
		validatorN := c.Validators[i]

		bech32, err := validatorN.AccountKeyBech32(ctx, valKey)
		if err != nil {
			return err
		}
		if err := validator0.AddGenesisAccount(ctx, bech32, genesisAmounts); err != nil {
			return err
		}

		if !c.Config().SkipGenTx {
			if err := validatorN.CopyGentx(ctx, validator0); err != nil {
				return err
			}
		}
	}

	if !c.Config().SkipGenTx {
		if err := validator0.CollectGentxs(ctx); err != nil {
			return err
		}
	}
	var outGenBz []byte
	if gensisContent != nil {
		outGenBz = gensisContent
	} else {
		for _, wallet := range additionalGenesisWallets {
			println("check wallet: ", wallet.Address)
			if err := validator0.AddGenesisAccount(ctx, wallet.Address, []sdk.Coin{{Denom: wallet.Denom, Amount: wallet.Amount}}); err != nil {
				return err
			}
		}

		genbz, err := validator0.GenesisFileContent(ctx)
		if err != nil {
			return err
		}

		genbz = bytes.ReplaceAll(genbz, []byte(`"stake"`), []byte(fmt.Sprintf(`"%s"`, chainCfg.Denom)))

		if c.Config().ModifyGenesis != nil {
			genbz, err = c.Config().ModifyGenesis(chainCfg, genbz)
			if err != nil {
				return err
			}
		}

		g := make(map[string]interface{})
		if err := json.Unmarshal(genbz, &g); err != nil {
			return fmt.Errorf("failed to unmarshal genesis file: %w", err)
		}

		if c.CosmosChain.Config().Bech32Prefix == "ethm" {
			// Add balance to hub genesis module account
			bankBalancesData, err := dyno.Get(g, "app_state", "bank", "balances")
			if err != nil {
				return fmt.Errorf("failed to retrieve bank balances: %w", err)
			}
			hubgenesisBalance := map[string]interface{}{
				"address": "ethm1748tamme3jj3v9wq95fc3pmglxtqscljdy7483",
				"coins": []interface{}{
					map[string]interface{}{
						"denom":  chainCfg.Denom,
						"amount": dymension.GenesisEventAmount.String(),
					},
				},
			}

			newBankBalances := append(bankBalancesData.([]interface{}), hubgenesisBalance)
			if err := dyno.Set(g, newBankBalances, "app_state", "bank", "balances"); err != nil {
				return fmt.Errorf("failed to set bank balances in genesis json: %w", err)
			}

			// Update supply for chain denom
			bankSupplyAmount, err := dyno.Get(g, "app_state", "bank", "supply", 0, "amount")
			if err != nil {
				return fmt.Errorf("failed to retrieve bank supply: %w", err)
			}
			amount, ok := sdkmath.NewIntFromString(bankSupplyAmount.(string))
			if !ok {
				return fmt.Errorf("failed to parse bank supply amount: %s", bankSupplyAmount)
			}
			newBankSupplyAmount := amount.Add(dymension.GenesisEventAmount)
			if err := dyno.Set(g, newBankSupplyAmount.String(), "app_state", "bank", "supply", 0, "amount"); err != nil {
				return fmt.Errorf("failed to set bank supply in genesis json: %w", err)
			}
		}

		outGenBz, err = json.Marshal(g)
		if err != nil {
			return fmt.Errorf("failed to marshal genesis bytes to json: %w", err)
		}
		// Provide EXPORT_GENESIS_FILE_PATH and EXPORT_GENESIS_CHAIN to help debug genesis file
		exportGenesis := os.Getenv("EXPORT_GENESIS_FILE_PATH")
		exportGenesisChain := os.Getenv("EXPORT_GENESIS_CHAIN")
		if exportGenesis != "" && exportGenesisChain == c.Config().Name {
			c.Logger().Debug("Exporting genesis file",
				zap.String("chain", exportGenesisChain),
				zap.String("path", exportGenesis),
			)
			_ = os.WriteFile(exportGenesis, outGenBz, 0600)
		}
	}
	nodes := c.Nodes()

	for _, node := range nodes {
		if err := node.OverwriteGenesisFile(ctx, outGenBz); err != nil {
			return err
		}
	}

	// Use validator to show sequencer key, so that it gets recognized as sequencer
	var command []string
	command = append(command, "dymint", "show-sequencer")
	seq, _, err := c.Validators[0].ExecBin(ctx, command...)
	c.sequencerKey = string(bytes.TrimSuffix(seq, []byte("\n")))

	if err != nil {
		return fmt.Errorf("failed to show seq %s: %w", c.Config().Name, err)
	}

	return nil
}

func (c *DymRollApp) ShowSequencer(ctx context.Context) (string, error) {
	var command []string
	command = append(command, "dymint", "show-sequencer")

	seq, _, err := c.GetNode().ExecBin(ctx, command...)
	return string(bytes.TrimSuffix(seq, []byte("\n"))), err
}

func (c *DymRollApp) GetSequencer() string {
	return c.sequencerKey
}

func (c *DymRollApp) GetSequencerKeyDir() string {
	fmt.Println("rollapp: ", c.GetChainID())
	fmt.Println("sequencerKeyDir: ", c.sequencerKey)
	return c.sequencerKeyDir
}

func (c *DymRollApp) InitValidatorGenTx(
	ctx context.Context,
	validator *cosmos.Node,
	validatorIdx int,
	chainConfig *ibc.ChainConfig,
	genesisAmounts []sdk.Coin,
	genesisSelfDelegation sdk.Coin,
) error {
	if err := validator.CreateKey(ctx, valKey); err != nil {
		return err
	}
	bech32, err := validator.AccountKeyBech32(ctx, valKey)
	if err != nil {
		return err
	}
	if err := validator.AddGenesisAccount(ctx, bech32, genesisAmounts); err != nil {
		return err
	}

	if validatorIdx == 0 {
		genbz, err := validator.GenesisFileContent(ctx)
		if err != nil {
			return err
		}

		valBech32, err := validator.KeyBech32(ctx, valKey, "val")
		if err != nil {
			return fmt.Errorf("failed to retrieve val bech32: %w", err)
		}

		g := make(map[string]interface{})
		if err := json.Unmarshal(genbz, &g); err != nil {
			return fmt.Errorf("failed to unmarshal genesis file: %w", err)
		}

		if err := dyno.Set(g, valBech32, "app_state", "sequencers", "genesis_operator_address"); err != nil {
			return fmt.Errorf("failed to set genesis operator address in genesis json: %w", err)
		}

		fmt.Println("genesis_operator_address", valBech32)

		outGenBz, err := json.Marshal(g)
		if err != nil {
			return fmt.Errorf("failed to marshal genesis bytes to json: %w", err)
		}

		if err := validator.OverwriteGenesisFile(ctx, outGenBz); err != nil {
			return err
		}
	}
	return validator.Gentx(ctx, valKey, genesisSelfDelegation)
}

func (c *DymRollApp) StartRollAppWithExitsHub(ctx context.Context) error {
	return fmt.Errorf("not implemented")
}
