package dymension

type EibcEvent struct {
	ID           string `json:"id"`
	Price        string `json:"price"`
	Fee          string `json:"fee"`
	IsFulfilled  bool   `json:"is_fulfilled"`
	PacketStatus string `json:"packet_status"`
}
