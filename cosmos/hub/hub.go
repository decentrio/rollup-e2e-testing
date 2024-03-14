package hub

import (
	"strings"

	"github.com/decentrio/rollup-e2e-testing/cosmos/hub/celes_hub"
	"github.com/decentrio/rollup-e2e-testing/cosmos/hub/dym_hub"
	"github.com/decentrio/rollup-e2e-testing/ibc"
	"go.uber.org/zap"
)

func NewHub(testName string, chainConfig ibc.ChainConfig, numValidators int, numFullNodes int, log *zap.Logger) ibc.Chain {
	chainType := strings.Split(chainConfig.Type, "-")

	if chainType[1] == "dym" {
		return dym_hub.NewDymHub(testName, chainConfig, numValidators, numFullNodes, log)
	} else if chainType[1] == "celes" {
		return celes_hub.NewCelesHub(testName, chainConfig, numValidators, numFullNodes, log)
	}

	return nil
}
