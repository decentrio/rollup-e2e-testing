package example

import (
	"context"
	"testing"

	"cosmossdk.io/math"
	transfertypes "github.com/cosmos/ibc-go/v8/modules/apps/transfer/types"
	test "github.com/decentrio/rollup-e2e-testing"
	"github.com/decentrio/rollup-e2e-testing/cosmos"
	"github.com/decentrio/rollup-e2e-testing/cosmos/hub/celes_hub"
	"github.com/decentrio/rollup-e2e-testing/cosmos/rollapp/gm_rollapp"
	"github.com/decentrio/rollup-e2e-testing/ibc"
	"github.com/decentrio/rollup-e2e-testing/relayer"
	"github.com/decentrio/rollup-e2e-testing/testreporter"
	"github.com/decentrio/rollup-e2e-testing/testutil"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

var rlyPath = "hub-gm"

// TestRollkitIBCTransfer is a test to checking ibc transfer working for GM chain and other cosmos chain
func TestRollkitIBCTransfer(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	ctx := context.Background()

	configFileOverrides := make(map[string]any)
	configTomlOverrides := make(testutil.Toml)
	configTomlOverrides["timeout_commit"] = "2s"
	configTomlOverrides["timeout_propose"] = "2s"
	configTomlOverrides["index_all_keys"] = "true"
	configTomlOverrides["mode"] = "validator"

	configFileOverrides["config/config.toml"] = configTomlOverrides
	// Create chain factory
	numHubVals := 1
	numHubFullNodes := 0
	numRollAppFn := 0
	numRollAppVals := 1
	cf := test.NewBuiltinChainFactory(zaptest.NewLogger(t), []*test.ChainSpec{
		{
			Name: "gm",
			ChainConfig: ibc.ChainConfig{
				Type:    "rollapp-gm",
				Name:    "gm",
				ChainID: "gm1",
				Images: []ibc.DockerImage{
					{
						Repository: "ghcr.io/decentrio/gm",
						Version:    "debug",
						UidGid:     "1025:1025",
					},
				},
				Bin:                 "gmd",
				Bech32Prefix:        "gm",
				Denom:               "stake",
				CoinType:            "118",
				GasPrices:           "0.0stake",
				GasAdjustment:       1.1,
				TrustingPeriod:      "112h",
				NoHostMount:         false,
				ModifyGenesis:       nil,
				ConfigFileOverrides: nil,
			},
			NumValidators: &numRollAppVals,
			NumFullNodes:  &numRollAppFn,
		},
		{
			Name: "celes-hub",
			ChainConfig: ibc.ChainConfig{
				Name:           "celestia",
				Denom:          "utia",
				Type:           "hub-celes",
				GasPrices:      "0.002utia",
				TrustingPeriod: "112h",
				ChainID:        "test",
				Bin:            "celestia-appd",
				Images: []ibc.DockerImage{
					{
						Repository: "ghcr.io/decentrio/celestia",
						Version:    "debug",
						UidGid:     "1025:1025",
					},
				},
				Bech32Prefix:        "celestia",
				CoinType:            "118",
				GasAdjustment:       1.5,
				ConfigFileOverrides: configFileOverrides,
			},
			NumValidators: &numHubVals,
			NumFullNodes:  &numHubFullNodes,
		},
		{
			Name:          "gaia",
			Version:       "v15.1.0",
			ChainConfig:   gaiaConfig,
			NumValidators: &numHubVals,
			NumFullNodes:  &numHubFullNodes,
		},
	})

	// Get chains from the chain factory
	chains, err := cf.Chains(t.Name())
	require.NoError(t, err)

	gm1 := chains[0].(*gm_rollapp.GmRollApp)
	celestia := chains[1].(*celes_hub.CelesHub)
	gaia := chains[2].(*cosmos.CosmosChain)

	// Relayer Factory
	client, network := test.DockerSetup(t)

	r := test.NewBuiltinRelayerFactory(ibc.CosmosRly, zaptest.NewLogger(t),
		relayer.CustomDockerImage("ghcr.io/cosmos/relayer", "v2.4.2", "100:1000"),
	).Build(t, client, "relayer", network)

	ic := test.NewSetup().
		AddRollUp(celestia, gm1).
		AddChain(gaia).
		AddRelayer(r, "relayer").
		AddLink(test.InterchainLink{
			Chain1:  gaia,
			Chain2:  gm1,
			Relayer: r,
			Path:    rlyPath,
		})

	rep := testreporter.NewNopReporter()
	eRep := rep.RelayerExecReporter(t)

	err = ic.Build(ctx, eRep, test.InterchainBuildOptions{
		TestName:         t.Name(),
		Client:           client,
		NetworkID:        network,
		SkipPathCreation: false,

		// This can be used to write to the block database which will index all block data e.g. txs, msgs, events, etc.
		// BlockDatabaseFile: test.DefaultBlockDatabaseFilepath(),
	})
	require.NoError(t, err)

	walletAmount := math.NewInt(1_000_000_000_000)

	// Create some user accounts on both chains
	users := test.GetAndFundTestUsers(t, ctx, t.Name(), walletAmount, celestia, gm1, gaia)

	// Wait a few blocks for relayer to start and for user accounts to be created
	err = testutil.WaitForBlocks(ctx, 5, celestia, gm1)
	require.NoError(t, err)

	// Get our Bech32 encoded user addresses
	celesUser, gmUser, gaiaUser := users[0], users[1], users[2]

	celesUserAddr := celesUser.FormattedAddress()
	gmUserAddr := gmUser.FormattedAddress()
	gaiaUserAddr := gaiaUser.FormattedAddress()

	// Get original account balances
	celesOrigBal, err := celestia.GetBalance(ctx, celesUserAddr, celestia.Config().Denom)
	require.NoError(t, err)
	require.Equal(t, walletAmount, celesOrigBal)

	gmOrigBal, err := gm1.GetBalance(ctx, gmUserAddr, gm1.Config().Denom)
	require.NoError(t, err)
	require.Equal(t, walletAmount, gmOrigBal)

	gaiaOrigBal, err := gaia.GetBalance(ctx, gaiaUserAddr, gaia.Config().Denom)
	require.NoError(t, err)
	require.Equal(t, walletAmount, gaiaOrigBal)

	// Compose an IBC transfer and send from gm -> hub
	var transferAmount = math.NewInt(1_000_000)

	channel, err := ibc.GetTransferChannel(ctx, r, eRep, gm1.Config().ChainID, gaia.Config().ChainID)
	require.NoError(t, err)

	transferData := ibc.WalletData{
		Address: gaiaUserAddr,
		Denom:   gm1.Config().Denom,
		Amount:  transferAmount,
	}

	_, err = gm1.SendIBCTransfer(ctx, channel.ChannelID, gmUserAddr, transferData, ibc.TransferOptions{})
	require.NoError(t, err)

	err = r.StartRelayer(ctx, eRep, rlyPath)
	require.NoError(t, err)

	t.Cleanup(
		func() {
			err := r.StopRelayer(ctx, eRep)
			if err != nil {
				t.Logf("an error occurred while stopping the relayer: %s", err)
			}
		},
	)

	err = testutil.WaitForBlocks(ctx, 10, gm1, gaia)
	require.NoError(t, err)

	// Get the IBC denom for urax on Hub
	gmTokenDenom := transfertypes.GetPrefixedDenom(channel.Counterparty.PortID, channel.Counterparty.ChannelID, gm1.Config().Denom)
	gmIBCDenom := transfertypes.ParseDenomTrace(gmTokenDenom).IBCDenom()

	// Assert balance was updated on the gm and gaia wallet 
	testutil.AssertBalance(t, ctx, gm1, gmUserAddr, gm1.Config().Denom, walletAmount.Sub(transferData.Amount))
	testutil.AssertBalance(t, ctx, gaia, gaiaUserAddr, gmIBCDenom, transferData.Amount)
}
