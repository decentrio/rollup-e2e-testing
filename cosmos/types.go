package cosmos

import (
	"encoding/json"
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
