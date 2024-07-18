package cosmos

import (
	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module/testutil"
	authTx "github.com/cosmos/cosmos-sdk/x/auth/tx"
	"github.com/cosmos/cosmos-sdk/x/bank"
	"github.com/cosmos/ibc-go/v7/modules/apps/transfer"
	ibctypes "github.com/cosmos/ibc-go/v7/modules/core"
)

func DefaultEncoding() testutil.TestEncodingConfig {
	return testutil.MakeTestEncodingConfig(
		// external modules
		bank.AppModuleBasic{},
		ibctypes.AppModuleBasic{},
		transfer.AppModuleBasic{},
	)
}

func decodeTX(interfaceRegistry codectypes.InterfaceRegistry, txbz []byte) (sdk.Tx, error) {
	cdc := codec.NewProtoCodec(interfaceRegistry)
	return authTx.DefaultTxDecoder(cdc)(txbz)
}

func encodeTxToJSON(interfaceRegistry codectypes.InterfaceRegistry, tx sdk.Tx) ([]byte, error) {
	cdc := codec.NewProtoCodec(interfaceRegistry)
	return authTx.DefaultJSONTxEncoder(cdc)(tx)
}
