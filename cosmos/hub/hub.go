package hub

import (
	"github.com/decentrio/rollup-e2e-testing/cosmos/hub/dyms_hub"
	"github.com/decentrio/rollup-e2e-testing/ibc"
	"go.uber.org/zap"
)

func NewHub(testName string, chainConfig ibc.ChainConfig, numValidators int, numFullNodes int, log *zap.Logger) ibc.Chain {
	return dyms_hub.NewDymsHub(testName, chainConfig, numValidators, numFullNodes, log)
}
