package cosmos

import (
	"encoding/json"

	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
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
	Height int64
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
	Height      int64
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

type QueryDenomsMetadataResponse struct {
	// metadata provides the client information for all the registered tokens.
	Metadatas []DenomMetadata `protobuf:"bytes,1,rep,name=metadatas,proto3" json:"metadatas"`
	// pagination defines the pagination in the response.
	Pagination *PageResponse `protobuf:"bytes,2,opt,name=pagination,proto3" json:"pagination,omitempty"`
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

type QueryClientStatusResponse struct {
	Status string `protobuf:"bytes,1,opt,name=status,proto3" json:"status,omitempty"`
}
type QuerySequencersResponse struct {
	Sequencers []Sequencer   `protobuf:"bytes,1,rep,name=sequencers,proto3" json:"sequencers"`
	Pagination *PageResponse `protobuf:"bytes,2,opt,name=pagination,proto3" json:"pagination,omitempty"`
}

// Sequencer defines a sequencer identified by its' address (sequencerAddress).
// The sequencer could be attached to only one rollapp (rollappId).
type Sequencer struct {
	// address is the bech32-encoded address of the sequencer account which is the account that the message was sent from.
	Address string `protobuf:"bytes,1,opt,name=address,proto3" json:"address,omitempty"`
	// pubkey is the public key of the sequencers' dymint client, as a Protobuf Any.
	DymintPubKey *codectypes.Any `protobuf:"bytes,2,opt,name=dymintPubKey,proto3" json:"dymintPubKey,omitempty"`
	// rollappId defines the rollapp to which the sequencer belongs.
	RollappId string `protobuf:"bytes,3,opt,name=rollappId,proto3" json:"rollappId,omitempty"`
	// metadata defines the extra information for the sequencer.
	Metadata SequencerMetadata `protobuf:"bytes,4,opt,name=metadata,proto3" json:"metadata"`
	// jailed defined whether the sequencer has been jailed from bonded status or not.
	Jailed bool `protobuf:"varint,5,opt,name=jailed,proto3" json:"jailed,omitempty"`
	// proposer defines whether the sequencer is a proposer or not.
	Proposer bool `protobuf:"varint,6,opt,name=proposer,proto3" json:"proposer,omitempty"`
	// status is the sequencer status (bonded/unbonding/unbonded).
	Status string `protobuf:"varint,7,opt,name=status,proto3,enum=dymensionxyz.dymension.sequencer.OperatingStatus" json:"status,omitempty"`
	// tokens define the delegated tokens (incl. self-delegation).
	Tokens types.Coins `protobuf:"bytes,8,rep,name=tokens,proto3,castrepeated=github.com/cosmos/cosmos-sdk/types.Coins" json:"tokens"`
	// unbonding_height defines, if unbonding, the height at which this sequencer has begun unbonding.
	UnbondingHeight string `protobuf:"varint,9,opt,name=unbonding_height,json=unbondingHeight,proto3" json:"unbonding_height,omitempty"`
	// unbond_time defines, if unbonding, the min time for the sequencer to complete unbonding.
	UnbondTime string `protobuf:"bytes,10,opt,name=unbond_time,json=unbondTime,proto3,stdtime" json:"unbond_time"`
}

// Metadata defines rollapp/sequencer extra information.
type SequencerMetadata struct {
	// moniker defines a human-readable name for the sequencer.
	Moniker string `protobuf:"bytes,1,opt,name=moniker,proto3" json:"moniker,omitempty"`
	// details define other optional details.
	Details string `protobuf:"bytes,5,opt,name=details,proto3" json:"details,omitempty"`
	// bootstrap nodes list
	P2PSeeds []string `protobuf:"bytes,6,rep,name=p2p_seeds,json=p2pSeeds,proto3" json:"p2p_seeds,omitempty"`
	// RPCs list
	Rpcs []string `protobuf:"bytes,7,rep,name=rpcs,proto3" json:"rpcs,omitempty"`
	// evm RPCs list
	EvmRpcs []string `protobuf:"bytes,8,rep,name=evm_rpcs,json=evmRpcs,proto3" json:"evm_rpcs,omitempty"`
	// REST API URLs
	RestApiUrls []string `protobuf:"bytes,9,rep,name=rest_api_urls,json=restApiUrls,proto3" json:"rest_api_urls,omitempty"`
	// block explorer URL
	ExplorerUrl string `protobuf:"bytes,10,opt,name=explorer_url,json=explorerUrl,proto3" json:"explorer_url,omitempty"`
	// genesis URLs
	GenesisUrls []string `protobuf:"bytes,11,rep,name=genesis_urls,json=genesisUrls,proto3" json:"genesis_urls,omitempty"`
	// contact details
	ContactDetails *ContactDetails `protobuf:"bytes,12,opt,name=contact_details,json=contactDetails,proto3" json:"contact_details,omitempty"`
	// json dump the sequencer can add (limited by size)
	ExtraData []byte `protobuf:"bytes,13,opt,name=extra_data,json=extraData,proto3" json:"extra_data,omitempty"`
	// snapshots of the sequencer
	Snapshots []*SnapshotInfo `protobuf:"bytes,14,rep,name=snapshots,proto3" json:"snapshots,omitempty"`
	// gas_price defines the value for each gas unit
	GasPrice *types.Int `protobuf:"bytes,15,opt,name=gas_price,json=gasPrice,proto3,customtype=github.com/cosmos/cosmos-sdk/types.Int" json:"gas_price,omitempty"`
}

type ContactDetails struct {
	// website URL
	Website string `protobuf:"bytes,11,opt,name=website,proto3" json:"website,omitempty"`
	// telegram link
	Telegram string `protobuf:"bytes,1,opt,name=telegram,proto3" json:"telegram,omitempty"`
	// twitter link
	X string `protobuf:"bytes,2,opt,name=x,proto3" json:"x,omitempty"`
}

type SnapshotInfo struct {
	// the snapshot url
	SnapshotUrl string `protobuf:"bytes,1,opt,name=snapshot_url,json=snapshotUrl,proto3" json:"snapshot_url,omitempty"`
	// The snapshot height
	Height uint64 `protobuf:"varint,2,opt,name=height,proto3" json:"height,omitempty"`
	// sha-256 checksum value for the snapshot file
	Checksum string `protobuf:"bytes,3,opt,name=checksum,proto3" json:"checksum,omitempty"`
}

type TokenPair struct {
	// erc20_address is the hex address of ERC20 contract token
	Erc20Address string `protobuf:"bytes,1,opt,name=erc20_address,json=erc20Address,proto3" json:"erc20_address,omitempty"`
	// denom defines the cosmos base denomination to be mapped to
	Denom string `protobuf:"bytes,2,opt,name=denom,proto3" json:"denom,omitempty"`
	// enabled defines the token mapping enable status
	Enabled bool `protobuf:"varint,3,opt,name=enabled,proto3" json:"enabled,omitempty"`
	// contract_owner is the an ENUM specifying the type of ERC20 owner (0 invalid, 1 ModuleAccount, 2 external address)
	ContractOwner string `protobuf:"varint,4,opt,name=contract_owner,json=contractOwner,proto3,enum=evmos.erc20.v1.Owner" json:"contract_owner,omitempty"`
}
type Erc20TokenPairResponse struct {
	// token_pairs returns the info about a registered token pair for the erc20 module
	TokenPair TokenPair `protobuf:"bytes,1,opt,name=token_pair,json=tokenPair,proto3" json:"token_pair"`
}

type DelayedACKParams struct {
	EpochIdentifier string    `protobuf:"bytes,1,opt,name=epoch_identifier,json=epochIdentifier,proto3" json:"epoch_identifier,omitempty" yaml:"epoch_identifier"`
	BridgingFee     types.Dec `protobuf:"bytes,2,opt,name=bridging_fee,json=bridgingFee,proto3,customtype=github.com/cosmos/cosmos-sdk/types.Dec" json:"bridging_fee" yaml:"bridging_fee"`
}

type CelestiaResponse struct {
	Result CelestiaBlockResult `json:"result"`
}

type CelestiaBlockResult struct {
	Block CelestiaBlock `json:"block"`
}

type CelestiaBlock struct {
	Header CelestiaBlockHeader `json:"header"`
}

type CelestiaBlockHeader struct {
	Height string `json:"height"`
}