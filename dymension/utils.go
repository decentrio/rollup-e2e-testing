package dymension

import (
	"encoding/base64"
	"fmt"
	"strconv"

	"github.com/decentrio/rollup-e2e-testing/blockdb"
)

func MapToEibcEvent(event blockdb.Event) (EibcEvent, error) {
	var eibcEvent EibcEvent

	for _, attr := range event.Attributes {
		decodedKey, err := base64.StdEncoding.DecodeString(attr.Key)
		if err != nil {
			return EibcEvent{}, fmt.Errorf("error decoding key: %w", err)
		}

		decodedValue, err := base64.StdEncoding.DecodeString(attr.Value)
		if err != nil {
			return EibcEvent{}, fmt.Errorf("error decoding value: %w", err)
		}

		switch string(decodedKey) {
		case "id":
			eibcEvent.ID = string(decodedValue)
		case "price":
			eibcEvent.Price = string(decodedValue)
		case "fee":
			eibcEvent.Fee = string(decodedValue)
		case "is_fulfilled":
			isFulfilled, err := strconv.ParseBool(string(decodedValue))
			if err != nil {
				return EibcEvent{}, fmt.Errorf("error parsing boolean: %w", err)
			}
			eibcEvent.IsFulfilled = isFulfilled
		case "packet_status":
			eibcEvent.PacketStatus = string(decodedValue)
		}
	}

	return eibcEvent, nil
}
