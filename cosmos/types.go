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

	// IBC-Go <= v7 / SDK <= v0.47
	ProposalStatusUnspecified   = "PROPOSAL_STATUS_UNSPECIFIED"
	ProposalStatusPassed        = "PROPOSAL_STATUS_PASSED"
	ProposalStatusFailed        = "PROPOSAL_STATUS_FAILED"
	ProposalStatusRejected      = "PROPOSAL_STATUS_REJECTED"
	ProposalStatusVotingPeriod  = "PROPOSAL_STATUS_VOTING_PERIOD"
	ProposalStatusDepositPeriod = "PROPOSAL_STATUS_DEPOSIT_PERIOD"
)

// TxProposalv1 contains chain proposal transaction detail for gov module v1 (sdk v0.46.0+)
type TxProposalv1 struct {
	Messages []json.RawMessage `json:"messages"`
	Metadata string            `json:"metadata"`
	Deposit  string            `json:"deposit"`
	Title    string            `json:"title"`
	Summary  string            `json:"summary"`

	// SDK v50 only
	Proposer  string `json:"proposer,omitempty"`
	Expedited bool   `json:"expedited,omitempty"`
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

type ProposalMessage struct {
	Type  string `json:"type"`
	Value struct {
		Sender           string `json:"sender"`
		ValidatorAddress string `json:"validator_address"`
		Power            string `json:"power"`
		Unsafe           bool   `json:"unsafe"`
	} `json:"value"`
}

type ProposalContent struct {
	Type        string `json:"@type"`
	Title       string `json:"title"`
	Description string `json:"description"`
}

type ProposalFinalTallyResult struct {
	Yes        string `json:"yes_count"`
	Abstain    string `json:"abstain_count"`
	No         string `json:"no_count"`
	NoWithVeto string `json:"no_with_veto_count"`
}

type ProposalDeposit struct {
	Denom  string `json:"denom"`
	Amount string `json:"amount"`
}

type ParamChange struct {
	Subspace string `json:"subspace"`
	Key      string `json:"key"`
	Value    any    `json:"value"`
}

type ContractStateModels struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type BuildDependency struct {
	Parent  string `json:"parent"`
	Version string `json:"version"`

	IsReplacement      bool   `json:"is_replacement"`
	Replacement        string `json:"replacement"`
	ReplacementVersion string `json:"replacement_version"`
}

type BinaryBuildInformation struct {
	Name             string            `json:"name"`
	ServerName       string            `json:"server_name"`
	Version          string            `json:"version"`
	Commit           string            `json:"commit"`
	BuildTags        string            `json:"build_tags"`
	Go               string            `json:"go"`
	BuildDeps        []BuildDependency `json:"build_deps"`
	CosmosSdkVersion string            `json:"cosmos_sdk_version"`
}

type BankMetaData struct {
	Metadata struct {
		Description string `json:"description"`
		DenomUnits  []struct {
			Denom    string   `json:"denom"`
			Exponent int      `json:"exponent"`
			Aliases  []string `json:"aliases"`
		} `json:"denom_units"`
		Base    string `json:"base"`
		Display string `json:"display"`
		Name    string `json:"name"`
		Symbol  string `json:"symbol"`
		URI     string `json:"uri"`
		URIHash string `json:"uri_hash"`
	} `json:"metadata"`
}

type QueryModuleAccountResponse struct {
	Account struct {
		BaseAccount struct {
			AccountNumber string `json:"account_number"`
			Address       string `json:"address"`
			PubKey        string `json:"pub_key"`
			Sequence      string `json:"sequence"`
		} `json:"base_account"`
		Name string `json:"name"`
	} `json:"account"`
}

type QuerySequencersResponse struct {
	Sequencers []Sequencer   `protobuf:"bytes,1,rep,name=sequencers,proto3" json:"sequencers"`
	Pagination *PageResponse `protobuf:"bytes,2,opt,name=pagination,proto3" json:"pagination,omitempty"`
}

// Sequencer defines a sequencer identified by its' address (sequencerAddress).
// The sequencer could be attached to only one rollapp (rollappId).
type Sequencer struct {
	// sequencerAddress is the bech32-encoded address of the sequencer account which is the account that the message was sent from.
	SequencerAddress string `protobuf:"bytes,1,opt,name=sequencerAddress,proto3" json:"sequencerAddress,omitempty"`
	// pubkey is the public key of the sequencers' dymint client, as a Protobuf Any.
	DymintPubKey *codectypes.Any `protobuf:"bytes,2,opt,name=dymintPubKey,proto3" json:"dymintPubKey,omitempty"`
	// rollappId defines the rollapp to which the sequencer belongs.
	RollappId string `protobuf:"bytes,3,opt,name=rollappId,proto3" json:"rollappId,omitempty"`
	// description defines the descriptive terms for the sequencer.
	Description Description `protobuf:"bytes,4,opt,name=description,proto3" json:"description"`
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

// Description defines a sequencer description.
type Description struct {
	// moniker defines a human-readable name for the sequencer.
	Moniker string `protobuf:"bytes,1,opt,name=moniker,proto3" json:"moniker,omitempty"`
	// identity defines an optional identity signature (ex. UPort or Keybase).
	Identity string `protobuf:"bytes,2,opt,name=identity,proto3" json:"identity,omitempty"`
	// website defines an optional website link.
	Website string `protobuf:"bytes,3,opt,name=website,proto3" json:"website,omitempty"`
	// securityContact defines an optional email for security contact.
	SecurityContact string `protobuf:"bytes,4,opt,name=securityContact,proto3" json:"securityContact,omitempty"`
	// details define other optional details.
	Details string `protobuf:"bytes,5,opt,name=details,proto3" json:"details,omitempty"`
}

type PageResponse struct {
	NextKey string `json:"next_key"`
	Total   string `json:"total"`
}
