package dymension

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/decentrio/rollup-e2e-testing/blockdb"
	"github.com/decentrio/rollup-e2e-testing/dymension"
)

func MapToEibcEvent(event blockdb.Event) (EibcEvent, error) {
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

func WaitUntilRollappHeightIsFinalized(ctx context.Context, dymension *dymension.DymHub, rollappChainID string, targetHeight uint64, timeoutSecs int) (bool, error) {
	startTime := time.Now()
	timeout := time.Duration(timeoutSecs) * time.Second

	for {
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		case <-time.After(timeout):
			return false, fmt.Errorf("specified rollapp height %d not found within the timeout", targetHeight)
		default:
			rollappState, err := dymension.QueryRollappState(ctx, rollappChainID, true)
			if err != nil {
				if time.Since(startTime) < timeout {
					time.Sleep(2 * time.Second) // Wait for 2 seconds before retrying.
					continue                    // Retry the loop.
				} else {
					return false, fmt.Errorf("error querying rollapp state: %v", err)
				}
			}

			for _, bd := range rollappState.StateInfo.BlockDescriptors.BD {
				height, err := strconv.ParseUint(bd.Height, 10, 64)
				if err != nil {
					continue
				}
				if height == targetHeight {
					return true, nil
				}
			}

			if time.Since(startTime)+2*time.Second < timeout {
				time.Sleep(2 * time.Second)
			} else {
				return false, fmt.Errorf("specified rollapp height %d not found within the timeout", targetHeight)
			}
		}
	}
}
