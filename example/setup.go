package example

import (
	"os"
	"strings"

	"github.com/decentrio/rollup-e2e-testing/cosmos"
	"github.com/decentrio/rollup-e2e-testing/ibc"
	"github.com/cosmos/cosmos-sdk/types/module/testutil"
)

var (
	DymensionMainRepo   = "ghcr.io/dymensionxyz/dymension"
	DymensionICTestRepo = "ghcr.io/dymensionxyz/dymension-ictest"

	RollappMainRepo = "ghcr.io/decentrio/rollapp"

	repo, version = GetDockerImageInfo()

	dymensionImage = ibc.DockerImage{
		Repository: repo,
		Version:    version,
		UidGid:     "1025:1025",
	}

	// Setup for gaia
	gaiaImageRepo = "ghcr.io/strangelove-ventures/heighliner/gaia" //

	gaiaImage = ibc.DockerImage{
		Repository: gaiaImageRepo,
		UidGid:     "1025:1025",
	}

	dymensionConfig = ibc.ChainConfig{
		Type:                "hub-dyms",
		Name:                "dymension",
		ChainID:             "dymension_100-1",
		Images:              []ibc.DockerImage{dymensionImage},
		Bin:                 "dymd",
		Bech32Prefix:        "dym",
		Denom:               "udym",
		CoinType:            "118",
		GasPrices:           "0.0udym",
		EncodingConfig:      evmConfig(),
		GasAdjustment:       1.1,
		TrustingPeriod:      "112h",
		NoHostMount:         false,
		ModifyGenesis:       nil,
		ConfigFileOverrides: nil,
	}

	gaiaConfig = ibc.ChainConfig{
		Type:                "cosmos",
		Name:                "gaia",
		ChainID:             "gaia-1",
		Images:              []ibc.DockerImage{gaiaImage},
		Bin:                 "gaiad",
		Bech32Prefix:        "cosmos",
		Denom:               "uatom",
		CoinType:            "118",
		GasPrices:           "0uatom",
		EncodingConfig:      nil,
		GasAdjustment:       2,
		TrustingPeriod:      "112h",
		NoHostMount:         false,
		ModifyGenesis:       nil,
		ConfigFileOverrides: nil,
	}
)

// GetDockerImageInfo returns the appropriate repo and branch version string for integration with the CI pipeline.
// The remote runner sets the BRANCH_CI env var. If present, tests will use the docker image pushed up to the repo.
// If testing locally, user should run `make docker-build-debug` and tests will use the local image.
func GetDockerImageInfo() (repo, version string) {
	branchVersion, found := os.LookupEnv("BRANCH_CI")
	repo = DymensionICTestRepo
	if !found {
		// make local-image
		repo = "ghcr.io/decentrio/dymension"
		branchVersion = "e2e"
	}

	// github converts / to - for pushed docker images
	branchVersion = strings.ReplaceAll(branchVersion, "/", "-")
	return repo, branchVersion
}

func evmConfig() *testutil.TestEncodingConfig {
	cfg := cosmos.DefaultEncoding()

	return &cfg
}
