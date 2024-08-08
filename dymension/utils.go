package dymension

import (
	"encoding/base64"
	"fmt"
	"strconv"

	"github.com/decentrio/rollup-e2e-testing/blockdb"
)

func decodeBase64OrFallback(value string) string {
	decodedValue, err := base64.StdEncoding.DecodeString(value)
	// Return the original value if decoding fails
	if err != nil {
		return value
	}
	return string(decodedValue)
}

func MapToEibcEvent(event blockdb.Event) (EibcEvent, error) {
	var eibcEvent EibcEvent

	for _, attr := range event.Attributes {
		decodedKey := decodeBase64OrFallback(attr.Key)
		decodedValue := decodeBase64OrFallback(attr.Value)

		switch decodedKey {
		case "id":
			eibcEvent.ID = decodedValue
		case "price":
			eibcEvent.Price = decodedValue
		case "fee":
			eibcEvent.Fee = decodedValue
		case "is_fulfilled":
			isFulfilled, err := strconv.ParseBool(decodedValue)
			if err != nil {
				return EibcEvent{}, fmt.Errorf("error parsing boolean: %w", err)
			}
			eibcEvent.IsFulfilled = isFulfilled
		case "packet_status":
			eibcEvent.PacketStatus = decodedValue
		}
	}

	return eibcEvent, nil
}
