package dymension

import (
	"encoding/base64"
	"fmt"
	"strconv"

	"github.com/decentrio/rollup-e2e-testing/blockdb"
)

func isBase64(s string) bool {
	_, err := base64.StdEncoding.DecodeString(s)
	return err == nil
}

func MapToEibcEvent(event blockdb.Event) (EibcEvent, error) {
	// Check if attributes are not empty to avoid out-of-bounds error
	if len(event.Attributes) == 0 {
		return EibcEvent{}, fmt.Errorf("no attributes in event")
	}

	// Determine if attributes are base64 encoded
	isBase64Encoded := isBase64(event.Attributes[0].Key)

	if isBase64Encoded {
		return MapToEibcEventBase64(event)
	}

	return MapToEibcEventPlainStr(event)
}

func MapToEibcEventPlainStr(event blockdb.Event) (EibcEvent, error) {
	var eibcEvent EibcEvent

	for _, attr := range event.Attributes {
		switch attr.Key {
		case "id":
			eibcEvent.ID = attr.Value
		case "price":
			eibcEvent.Price = attr.Value
		case "fee":
			eibcEvent.Fee = attr.Value
		case "is_fulfilled":
			isFulfilled, err := strconv.ParseBool(attr.Value)
			if err != nil {
				return EibcEvent{}, err
			}
			eibcEvent.IsFulfilled = isFulfilled
		case "packet_status":
			eibcEvent.PacketStatus = attr.Value
		}
	}

	return eibcEvent, nil
}

func MapToEibcEventBase64(event blockdb.Event) (EibcEvent, error) {
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
