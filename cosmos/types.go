package cosmos

import (
	"encoding/json"
	"github.com/cosmos/cosmos-sdk/types"
)

const (
	ProposalVoteYes        = "yes"
	ProposalVoteNo         = "no"
	ProposalVoteNoWithVeto = "noWithVeto"
	ProposalVoteAbstain    = "abstain"

	ProposalStatusUnspecified   = "PROPOSAL_STATUS_UNSPECIFIED"
	ProposalStatusPassed        = "PROPOSAL_STATUS_PASSED"
	ProposalStatusFailed        = "PROPOSAL_STATUS_FAILED"
	ProposalStatusRejected      = "PROPOSAL_STATUS_REJECTED"
	ProposalStatusVotingPeriod  = "PROPOSAL_STATUS_VOTING_PERIOD"
	ProposalStatusDepositPeriod = "PROPOSAL_STATUS_DEPOSIT_PERIOD"
)

// ProtoMessage is implemented by generated protocol buffer messages.
// Pulled from github.com/cosmos/gogoproto/proto.
type ProtoMessage interface {
	Reset()
	String() string
	ProtoMessage()
}

// TxProposalv1 contains chain proposal transaction detail for gov module v1 (sdk v0.46.0+)
type TxProposalv1 struct {
	Messages []json.RawMessage `json:"messages"`
	Metadata string            `json:"metadata"`
	Deposit  string            `json:"deposit"`
	Title    string            `json:"title"`
	Summary  string            `json:"summary"`
}

// TxProposal contains chain proposal transaction details.
type TxProposal struct {
	// The block height.
	Height uint64
	// The transaction hash.
	TxHash string
	// Amount of gas charged to the account.
	GasSpent int64

	// Amount deposited for proposal.
	DepositAmount string
	// ID of proposal.
	ProposalID string
	// Type of proposal.
	ProposalType string
}

// SoftwareUpgradeProposal defines the required and optional parameters for submitting a software-upgrade proposal.
type TextProposal struct {
	Deposit     string
	Title       string
	Description string
	Expedited   bool
}

// SoftwareUpgradeProposal defines the required and optional parameters for submitting a software-upgrade proposal.
type SoftwareUpgradeProposal struct {
	Deposit     string
	Title       string
	Name        string
	Description string
	Height      uint64
	Info        string // optional
}

// ProposalResponse is the proposal query response.
type ProposalResponse struct {
	ProposalID       string                   `json:"proposal_id"`
	Content          ProposalContent          `json:"content"`
	Status           string                   `json:"status"`
	FinalTallyResult ProposalFinalTallyResult `json:"final_tally_result"`
	SubmitTime       string                   `json:"submit_time"`
	DepositEndTime   string                   `json:"deposit_end_time"`
	TotalDeposit     []ProposalDeposit        `json:"total_deposit"`
	VotingStartTime  string                   `json:"voting_start_time"`
	VotingEndTime    string                   `json:"voting_end_time"`
}

type ProposalContent struct {
	Type        string `json:"@type"`
	Title       string `json:"title"`
	Description string `json:"description"`
}

type ProposalFinalTallyResult struct {
	Yes        string `json:"yes"`
	Abstain    string `json:"abstain"`
	No         string `json:"no"`
	NoWithVeto string `json:"no_with_veto"`
}

type ProposalDeposit struct {
	Denom  string `json:"denom"`
	Amount string `json:"amount"`
}

type ParamChange struct {
	Subspace string `json:"subspace"`
	Key      string `json:"key"`
	Value    string `json:"value"`
}

type ContractStateModels struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type StateIndexResponse struct {
	StateIndex StateIndex `json:"stateIndex"`
}

type StateIndex struct {
	RollappID string `json:"rollappId"`
	Index     string `json:"index"`
}

type ModuleAccountResponse struct {
	Account ModuleAccount `json:"account"`
}

type ModuleAccount struct {
	Type        string      `json:"@type"`
	BaseAccount BaseAccount `json:"base_account"`
	Name        string      `json:"name"`
	Permissions []string    `json:"permissions"`
}

type BaseAccount struct {
	Address       string      `json:"address"`
	PubKey        interface{} `json:"pub_key"`
	AccountNumber string      `json:"account_number"`
	Sequence      string      `json:"sequence"`
}

type DenomMetadataResponse struct {
	Metadata DenomMetadata `json:"metadata"`
}

// DenomMetadata represents a struct that describes
// a basic token.
type DenomMetadata struct {
	Description string      `json:"description"`
	DenomUnits  []DenomUnit `json:"denom_units"`
	Base        string      `json:"base"`
	Display     string      `json:"display"`
	Name        string      `json:"name"`
	Symbol      string      `json:"symbol"`
	URI         string      `json:"uri"`
	URIHash     string      `json:"uri_hash"`
}

type DenomUnit struct {
	Denom    string   `json:"denom"`
	Exponent uint32   `json:"exponent"`
	Aliases  []string `json:"aliases"`
}

type Params struct {
	// send_enabled enables or disables all cross-chain token transfers from this
	// chain.
	SendEnabled bool `protobuf:"varint,1,opt,name=send_enabled,json=sendEnabled,proto3" json:"send_enabled,omitempty"`
	// receive_enabled enables or disables all cross-chain token transfers to this
	// chain.
	ReceiveEnabled bool `protobuf:"varint,2,opt,name=receive_enabled,json=receiveEnabled,proto3" json:"receive_enabled,omitempty"`
}

type QueryPacketCommitmentsResponse struct {
	Commitments []*PacketState `json:"commitments"`
	Pagination  *PageResponse  `json:"pagination"`
	Height      Height         `json:"height"`
}

type PacketState struct {
	// channel port identifier.
	PortId string `yaml:"port_id"`
	// channel unique identifier.
	ChannelId string `yaml:"channel_id"`
	// packet sequence.
	Sequence string `json:"sequence"`
	// embedded data that represents packet state.
	Data string `json:"data"`
}

type PageResponse struct {
	NextKey string `json:"next_key"`
	Total   string `json:"total"`
}

type Height struct {
	RevisionNumber string `yaml:"revision_number"`
	RevisionHeight string `yaml:"revision_height"`
}

type HubGenesisState struct {
	// is_locked is a boolean that indicates if the genesis event has occured
	IsLocked bool `protobuf:"varint,1,opt,name=is_locked,json=isLocked,proto3" json:"is_locked,omitempty"`
	// genesis_tokens is the list of tokens that are expected to be locked on genesis event
	GenesisTokens types.Coins `protobuf:"bytes,2,rep,name=genesis_tokens,json=genesisTokens,proto3,castrepeated=github.com/cosmos/cosmos-sdk/types.Coins" json:"genesis_tokens"`
}
