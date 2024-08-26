package dymension

import (
	"math"
	"time"

	sdkmath "cosmossdk.io/math"
	"github.com/gogo/protobuf/types"
)

var GenesisEventAmount = sdkmath.NewInt(100_000_000_000_000).MulRaw(int64(math.Pow10(6)))

type Status int32

const (
	Status_PENDING   Status = 0
	Status_FINALIZED Status = 1
	Status_REVERTED  Status = 3
)

var Status_value = map[string]int32{
	"PENDING":   0,
	"FINALIZED": 1,
	"REVERTED":  3,
}

type EibcEvent struct {
	OrderId         string `json:"order_id"`
	Price           string `json:"price"`
	Fee             string `json:"fee"`
	IsFulfilled     bool   `json:"is_fulfilled"`
	PacketStatus    string `json:"packet_status"`
	PacketKey       string `json:"packet_key"`
	RollAppId       string `json:"rollapp_id"`
	Recipient       string `json:"recipient"`
	NewPacketStatus Status `json:"new_packet_status"`
}

type RollappState struct {
	StateInfo StateInfo `json:"stateInfo"`
}

type StateInfo struct {
	StateInfoIndex   StateInfoIndex `json:"stateInfoIndex"`
	Sequencer        string         `json:"sequencer"`
	StartHeight      string         `json:"startHeight"`
	NumBlocks        string         `json:"numBlocks"`
	DAPath           string         `json:"DAPath"`
	Version          string         `json:"version"`
	CreationHeight   string         `json:"creationHeight"`
	Status           string         `json:"status"`
	BlockDescriptors BDs            `json:"BDs"`
}

type QueryGetRollappResponse struct {
	Rollapp                   Rollapp         `json:"rollapp"`
	LatestStateIndex          *StateInfoIndex `json:"latestStateIndex"`
	LatestFinalizedStateIndex *StateInfoIndex `json:"latestFinalizedStateIndex"`
}

type Rollapp struct {
	RollappId             string               `json:"rollappId"`
	Creator               string               `json:"creator"`
	Version               string               `json:"version"`
	CodeStamp             string               `json:"codeStamp"`            // Deprecated: Do not use.
	GenesisPath           string               `json:"genesisPath"`          // Deprecated: Do not use.
	MaxWithholdingBlocks  string               `json:"maxWithholdingBlocks"` // Deprecated: Do not use.
	MaxSequencers         string               `json:"maxSequencers"`
	PermissionedAddresses []string             `json:"permissionedAddresses"`
	TokenMetadata         []*TokenMetadata     `json:"tokenMetadata"`
	GenesisState          *RollappGenesisState `json:"genesis_state"`
	ChannelId             string               `json:"channel_id"`
	Frozen                bool                 `json:"frozen"`
}

type RollappGenesisState struct {
	GenesisAccounts []GenesisAccount `json:"genesis_accounts"`
	IsGenesisEvent  bool             `json:"is_genesis_event"`
}

type GenesisAccount struct {
	Amount  Coin   `json:"amount"`
	Address string `json:"address"`
}

type Coin struct {
	Denom  string `json:"denom"`
	Amount string `json:"amount"`
}

type TokenMetadata struct {
	Description string       `json:"description"`
	DenomUnits  []*DenomUnit `json:"denom_units"`
	Base        string       `json:"base"`
	Display     string       `json:"display"`
	Name        string       `json:"name"`
	Symbol      string       `json:"symbol"`
	URI         string       `json:"uri"`
	URIHash     string       `json:"uri_hash"`
}

type DenomUnit struct {
	Denom    string   `json:"denom"`
	Exponent uint64   `json:"exponent"`
	Aliases  []string `json:"aliases"`
}

type QueryGetLatestStateIndexResponse struct {
	StateIndex StateInfoIndex `json:"stateIndex"`
}

type StateInfoIndex struct {
	RollappId string `json:"rollappId"`
	Index     string `json:"index"`
}

type BDs struct {
	BD []BlockDescriptor `json:"BD"`
}

type BlockDescriptor struct {
	Height                 string `json:"height"`
	StateRoot              string `json:"stateRoot"`
	IntermediateStatesRoot string `json:"intermediateStatesRoot"`
}

type QueryDemandOrdersByStatusResponse struct {
	DemandOrders []*DemandOrder `json:"demand_orders"`
}

type DemandOrder struct {
	Id                   string `json:"id"`
	TrackingPacketKey    string `json:"tracking_packet_key"`
	Price                Coins  `json:"price"`
	Fee                  Coins  `json:"fee"`
	Recipient            string `json:"recipient"`
	IsFullfilled         bool   `json:"is_fullfilled"`
	TrackingPacketStatus string `json:"tracking_packet_status"`
}

type Coins []Coin

type QueryEpochsInfoResponse struct {
	Epochs []EpochInfo `json:"epochs"`
}

type EpochInfo struct {
	Identifier              string    `json:"identifier"`
	StartTime               time.Time `json:"start_time"`
	Duration                string    `json:"duration"`
	CurrentEpoch            string    `json:"current_epoch"`
	CurrentEpochStartTime   time.Time `json:"current_epoch_start_time"`
	EpochCountingStarted    bool      `json:"epoch_counting_started"`
	CurrentEpochStartHeight string    `json:"current_epoch_start_height"`
}

type QueryGetSequencersByRollappResponse struct {
	Sequencers []Sequencer `json:"sequencers"`
}

type Sequencer struct {
	SequencerAddress string      `json:"sequencerAddress"`
	DymintPubKey     *types.Any  `json:"dymintPubKey"`
	RollappId        string      `json:"rollappId"`
	Description      Description `json:"description"`
	Jailed           bool        `json:"jailed"`
	Proposer         bool        `json:"proposer"`
	Status           string      `json:"status"`
	Tokens           []Coin      `json:"tokens"`
	UnbondingHeight  string      `json:"unbonding_height"`
	UnbondTime       time.Time   `json:"unbond_time"`
}
type Description struct {
	Moniker         string `json:"moniker,omitempty"`
	Identity        string `json:"identity,omitempty"`
	Website         string `json:"website,omitempty"`
	SecurityContact string `json:"securityContact,omitempty"`
	Details         string `json:"details,omitempty"`
}

type QueryGetSequencerResponse struct {
	Sequencer Sequencer `json:"sequencer"`
}
