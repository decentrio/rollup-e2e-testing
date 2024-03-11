package dymension

import "encoding/json"

type EibcEvent struct {
	ID           string `json:"id"`
	Price        string `json:"price"`
	Fee          string `json:"fee"`
	IsFulfilled  bool   `json:"is_fulfilled"`
	PacketStatus string `json:"packet_status"`
}

type StateStatus int32

const (
	STATE_STATUS_UNSPECIFIED StateStatus = iota
	STATE_STATUS_RECEIVED
	STATE_STATUS_FINALIZED
)

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
	Status           StateStatus    `json:"status"`
	BlockDescriptors BDs            `json:"BDs"`
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

func (ss *StateStatus) UnmarshalJSON(data []byte) error {
	var status string
	if err := json.Unmarshal(data, &status); err != nil {
		return err
	}

	switch status {
	case "STATE_STATUS_RECEIVED":
		*ss = STATE_STATUS_RECEIVED
	case "STATE_STATUS_FINALIZED":
		*ss = STATE_STATUS_FINALIZED
	default:
		*ss = STATE_STATUS_UNSPECIFIED
	}
	return nil
}
