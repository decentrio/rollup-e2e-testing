package rollapp

import (
	dymsrollapp "github.com/decentrio/rollup-e2e-testing/cosmos/rollapp/dyms_rollapp"
	"github.com/decentrio/rollup-e2e-testing/ibc"
	"go.uber.org/zap"
)

func NewRollApp(testName string, chainConfig ibc.ChainConfig, numValidators int, numFullNodes int, log *zap.Logger) ibc.Chain {
	return dymsrollapp.NewDymsRollApp(testName, chainConfig, numValidators, numFullNodes, log)
}
