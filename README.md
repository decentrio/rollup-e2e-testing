<div align="center">
<h1><code>Rollup-e2e-testing</code></h1>
</div>

# Overview

Rollup-e2e-testing is designed as a framework for the purpose of testing rollup models such as Dymension, Rollkit, etc.

The framework is developed based on the architecture of [interchaintest](https://github.com/strangelove-ventures/interchaintest), [osmosis-e2e](https://github.com/osmosis-labs/osmosis/tree/main/tests/e2e), [gaia-e2e](https://github.com/cosmos/gaia/tree/main/tests/e2e),... to help quickly spin up custom testnets and dev environments to test IBC, [Relayer](https://github.com/cosmos/relayer) setup, hub and rollapp infrastructure, smart contracts, etc.

# Tutorial

Use Rollup-e2e-testing as a Module:

This document breaks down code snippets from [rollkit_test.go](../example/rollkit_test.go). This test:

1) Spins up GM, Celestia DA and Gaia
2) Creates a connection between GM and Celestia DA
2) Creates an IBC Path between GM and Gaia (client, connection, channel)
3) Sends an IBC transaction between GM and Gaia.

It then validates each step and confirms that the balances of each wallet are correct.

Three basic components of `rollup-e2e-testing`:

- **Chain Factory** - Select gm, celesita and gaia binaries to include in tests
- **Relayer Factory** - Select Relayer to use in tests
- **Setup** - Where the testnet is configured and spun up

### Chain Factory

```go
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
```
### Relayer Factory

```go
client, network := test.DockerSetup(t)

r := test.NewBuiltinRelayerFactory(ibc.CosmosRly, zaptest.NewLogger(t),
	relayer.CustomDockerImage("ghcr.io/cosmos/relayer", "v2.4.2", "100:1000"),
).Build(t, client, "relayer", network)
```

### Setup
We prep the "Setup" by adding chains, a relayer, and specifying which chains to create IBC paths for:
```go
var rlyPath = "hub-gm"
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
```
# Environment Variable

- `SHOW_CONTAINER_LOGS`: Controls whether container logs are displayed.

    - Set to `"always"` to show logs for both pass and fail.
    - Set to `"never"` to never show any logs.
    - Leave unset to show logs only for failed tests.

- `KEEP_CONTAINERS`: Prevents testnet cleanup after completion.

    - Set to any non-empty value to keep testnet containers alive.

- `CONTAINER_LOG_TAIL`: Specifies the number of lines to display from container logs. Defaults to 50 lines.

# Branches

|                               **Branch Name**                                | **IBC-Go** | **Cosmos-sdk** |
|:----------------------------------------------------------------------------:|:----------:|:--------------:|
|         [v6](https://github.com/decentrio/rollup-e2e-testing/tree/v6)        |     v6     |     v0.46      |
|     [main](https://github.com/decentrio/rollup-e2e-testing/tree/main)     |     v8     |     v0.50      |
|     [v8_rollkit](https://github.com/decentrio/rollup-e2e-testing/tree/v8_rollkit)     |     v8     |     v0.50      |

# Example

Send IBC transaction from GM <-> Hub and vice versa.
```
cd example
bash clean.sh
go test -race -v -run TestRollkitIBCTransfer .
```