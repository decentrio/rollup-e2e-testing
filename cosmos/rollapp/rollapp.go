package rollapp

import (
	"github.com/decentrio/rollup-e2e-testing/cosmos/rollapp/dyms"
	"github.com/decentrio/rollup-e2e-testing/ibc"
	"go.uber.org/zap"
)

func NewRollApp(testName string, chainConfig ibc.ChainConfig, numValidators int, numFullNodes int, log *zap.Logger) ibc.Chain {
	return dyms.NewDymsRollApp(testName, chainConfig, numValidators, numFullNodes, log)
}
