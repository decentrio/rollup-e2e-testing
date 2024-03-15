package dym_hub

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"

	sdkmath "cosmossdk.io/math"
	"github.com/cosmos/cosmos-sdk/types"

	"github.com/decentrio/rollup-e2e-testing/cosmos"
	"github.com/decentrio/rollup-e2e-testing/ibc"
	"github.com/decentrio/rollup-e2e-testing/testutil"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
)

type DymHub struct {
	*cosmos.CosmosChain
	rollApp    ibc.RollApp
	extraFlags map[string]interface{}
}

type GenesisAccount struct {
	Amount  types.Coin `json:"amount"`
	Address string     `json:"address"`
}

var _ ibc.Chain = (*DymHub)(nil)
var _ ibc.Hub = (*DymHub)(nil)

const (
	sequencerName = "sequencer"
	maxSequencers = "5"
	valKey        = "validator"
)

func NewDymHub(testName string, chainConfig ibc.ChainConfig, numValidators int, numFullNodes int, log *zap.Logger, extraFlags map[string]interface{}) *DymHub {
	cosmosChain := cosmos.NewCosmosChain(testName, chainConfig, numValidators, numFullNodes, log)

	c := &DymHub{
		CosmosChain: cosmosChain,
		extraFlags:  extraFlags,
	}

	return c
}

func (c *DymHub) Start(testName string, ctx context.Context, additionalGenesisWallets ...ibc.WalletData) error {
	// Start chain
	chainCfg := c.Config()

	decimalPow := int64(math.Pow10(int(*chainCfg.CoinDecimals)))

	genesisAmount := types.Coin{
		Amount: sdkmath.NewInt(100_000_000_000_000).MulRaw(decimalPow),
		Denom:  chainCfg.Denom,
	}

	genesisSelfDelegation := types.Coin{
		Amount: sdkmath.NewInt(50_000_000_000_000).MulRaw(decimalPow),
		Denom:  chainCfg.Denom,
	}

	if chainCfg.ModifyGenesisAmounts != nil {
		genesisAmount, genesisSelfDelegation = chainCfg.ModifyGenesisAmounts()
	}

	genesisAmounts := []types.Coin{genesisAmount}

	configFileOverrides := chainCfg.ConfigFileOverrides

	eg := new(errgroup.Group)
	// Initialize config and sign gentx for each validator.
	for _, v := range c.Validators {
		v := v
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
		return err
	}

	if chainCfg.PreGenesis != nil {
		err := chainCfg.PreGenesis(chainCfg)
		if err != nil {
			return err
		}
	}

	// for the validators we need to collect the gentxs and the accounts
	// to the first node's genesis file
	validator0 := c.Validators[0]
	bech32, err := validator0.AccountKeyBech32(ctx, valKey)
	if err != nil {
		return err
	}
	genesisAccounts := []GenesisAccount{
		{
			Amount:  genesisAmount,
			Address: bech32,
		},
	}
	for i := 1; i < len(c.Validators); i++ {
		validatorN := c.Validators[i]

		bech32, err := validatorN.AccountKeyBech32(ctx, valKey)
		if err != nil {
			return err
		}
		genesisAccounts = append(genesisAccounts, GenesisAccount{
			Amount:  genesisAmount,
			Address: bech32,
		})
		if err := validator0.AddGenesisAccount(ctx, bech32, genesisAmounts); err != nil {
			return err
		}

		if !chainCfg.SkipGenTx {
			if err := validatorN.CopyGentx(ctx, validator0); err != nil {
				return err
			}
		}
	}
	fileBz, err := json.MarshalIndent(genesisAccounts, "", "    ")
	if err != nil {
		return err
	}

	err = validator0.WriteFile(ctx, fileBz, "genesis_accounts.json")
	if err != nil {
		return err
	}
	fmt.Println("file saved to ", c.HomeDir()+"/genesis_accounts.json")

	for _, wallet := range additionalGenesisWallets {
		if err := validator0.AddGenesisAccount(ctx, wallet.Address, []types.Coin{{Denom: wallet.Denom, Amount: wallet.Amount}}); err != nil {
			return err
		}
	}

	if !chainCfg.SkipGenTx {
		if err := validator0.CollectGentxs(ctx); err != nil {
			return err
		}
	}

	genbz, err := validator0.GenesisFileContent(ctx)
	if err != nil {
		return err
	}

	genbz = bytes.ReplaceAll(genbz, []byte(`"stake"`), []byte(fmt.Sprintf(`"%s"`, chainCfg.Denom)))

	if chainCfg.ModifyGenesis != nil {
		genbz, err = chainCfg.ModifyGenesis(chainCfg, genbz)
		if err != nil {
			return err
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
			return err
		}
	}

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
	testutil.WaitForBlocks(ctx, 5, c.GetNode())
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

	hasFlagGenesisPath, ok := c.extraFlags["genesis-accounts-path"].(bool)
	flags := map[string]string{}
	if hasFlagGenesisPath && ok {
		flags["genesis-accounts-path"] = validator0.HomeDir() + "/genesis_accounts.json"
	}
	if err := c.RegisterRollAppToHub(ctx, sequencerName, rollAppChainID, maxSequencers, keyDir, flags); err != nil {
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
func (c *DymHub) RegisterRollAppToHub(ctx context.Context, keyName, rollappChainID, maxSequencers, keyDir string, flags map[string]string) error {
	return c.GetNode().RegisterRollAppToHub(ctx, keyName, rollappChainID, maxSequencers, keyDir, flags)
}

func (c *DymHub) SetRollApp(rollApp ibc.RollApp) {
	c.rollApp = rollApp
}

func (c *DymHub) GetRollApp() ibc.RollApp {
	return c.rollApp
}

func (c *DymHub) FullfillDemandOrder(ctx context.Context,
	id string,
	keyName string,
) (txhash string, _ error) {
	command := []string{
		"eibc",
		"fulfill-order", id,
	}
	return c.FullNodes[0].ExecTx(ctx, keyName, command...)
}
