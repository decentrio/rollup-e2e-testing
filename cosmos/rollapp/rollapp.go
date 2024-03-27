package rollapp

import (
	"strings"

	"github.com/decentrio/rollup-e2e-testing/cosmos/rollapp/dym_rollapp"
	"github.com/decentrio/rollup-e2e-testing/cosmos/rollapp/gm_rollapp"
	"github.com/decentrio/rollup-e2e-testing/ibc"
	"go.uber.org/zap"
)

func NewRollApp(testName string, chainConfig ibc.ChainConfig, numValidators int, numFullNodes int, log *zap.Logger, extraFlags map[string]interface{}) ibc.Chain {
	chainType := strings.Split(chainConfig.Type, "-")

	if chainType[1] == "dym" {
		return dym_rollapp.NewDymRollApp(testName, chainConfig, numValidators, numFullNodes, log, extraFlags)
	} else if chainType[1] == "gm" {
		return gm_rollapp.NewGmRollApp(testName, chainConfig, numValidators, numFullNodes, log)
	}

	return nil
}
