package example

import (
	"context"
	"testing"

	"cosmossdk.io/math"
	test "github.com/decentrio/rollup-e2e-testing"
	"github.com/decentrio/rollup-e2e-testing/cosmos/hub/celes_hub"
	"github.com/decentrio/rollup-e2e-testing/cosmos/rollapp/gm_rollapp"
	"github.com/decentrio/rollup-e2e-testing/ibc"
	"github.com/decentrio/rollup-e2e-testing/relayer"
	"github.com/decentrio/rollup-e2e-testing/testreporter"
	"github.com/decentrio/rollup-e2e-testing/testutil"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

// TestStart is a basic test to assert that spinning up a dymension network with 1 validator works properly.
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
	// Create chain factory with dymension
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
						Repository: "ghcr.io/rollkit/gm",
						Version:    "d908f4f",
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
				GasPrices:      "0utia",
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
	})

	// Get chains from the chain factory
	chains, err := cf.Chains(t.Name())
	require.NoError(t, err)

	gm1 := chains[0].(*gm_rollapp.GmRollApp)
	celestia := chains[1].(*celes_hub.CelesHub)

	// Relayer Factory
	client, network := test.DockerSetup(t)

	r := test.NewBuiltinRelayerFactory(ibc.CosmosRly, zaptest.NewLogger(t),

		relayer.CustomDockerImage("ghcr.io/cosmos/relayer", "v2.4.2", "100:1000"),
	).Build(t, client, "relayer", network)

	ic := test.NewSetup().
		AddRollUp(celestia, gm1).
		AddRelayer(r, "relayer").
		AddLink(test.InterchainLink{
			Chain1:  celestia,
			Chain2:  gm1,
			Relayer: r,
			Path:    ibcPath,
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
	users := test.GetAndFundTestUsers(t, ctx, t.Name(), walletAmount, celestia, gm1)

	// Wait a few blocks for relayer to start and for user accounts to be created
	err = testutil.WaitForBlocks(ctx, 5, celestia, gm1)
	require.NoError(t, err)

	// Get our Bech32 encoded user addresses
	celesUser, gmUser := users[0], users[1]

	celesUserAddr := celesUser.FormattedAddress()
	gmUserAddr := gmUser.FormattedAddress()

	// Get original account balances
	celesOrigBal, err := celestia.GetBalance(ctx, celesUserAddr, celestia.Config().Denom)
	require.NoError(t, err)
	require.Equal(t, walletAmount, celesOrigBal)

	gmOrigBal, err := gm1.GetBalance(ctx, gmUserAddr, gm1.Config().Denom)
	require.NoError(t, err)
	require.Equal(t, walletAmount, gmOrigBal)

}
