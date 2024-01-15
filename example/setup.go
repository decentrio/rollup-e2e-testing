package example

import (
	"os"
	"strings"

	"github.com/decentrio/rollup-e2e-testing/ibc"
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

	dymensionConfig = ibc.ChainConfig{
		Type:                "hub",
		Name:                "dymension",
		ChainID:             "dymension_100-1",
		Images:              []ibc.DockerImage{dymensionImage},
		Bin:                 "dymd",
		Bech32Prefix:        "dym",
		Denom:               "udym",
		CoinType:            "118",
		GasPrices:           "0.0udym",
		GasAdjustment:       1.1,
		TrustingPeriod:      "112h",
		NoHostMount:         false,
		ModifyGenesis:       nil,
		ConfigFileOverrides: nil,
	}
)

// GetDockerImageInfo returns the appropriate repo and branch version string for integration with the CI pipeline.
// The remote runner sets the BRANCH_CI env var. If present, interchaintest will use the docker image pushed up to the repo.
// If testing locally, user should run `make docker-build-debug` and interchaintest will use the local image.
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
