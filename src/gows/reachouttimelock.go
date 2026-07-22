package gows

import (
	"context"
	"encoding/json"

	"go.mau.fi/whatsmeow/types/events"
)

// The same MEX (GraphQL) query WhatsApp Web runs on startup
// (xwa2_fetch_account_reachout_timelock).
const queryFetchAccountReachoutTimelock = "23983697327930364"

type respFetchAccountReachoutTimelock struct {
	Timelock *events.NotifyAccountReachoutTimelock `json:"xwa2_fetch_account_reachout_timelock"`
}

// FetchAccountReachoutTimelock fetches the current state of the account's
// reachout timelock (the restriction WhatsApp places on reaching out to new
// contacts). It's the active-fetch counterpart of the
// events.NotifyAccountReachoutTimelock push notification.
// If the response contains no timelock data, a zero-value event
// (IsActive false) is returned instead of an error.
func (gows *GoWS) FetchAccountReachoutTimelock(ctx context.Context) (*events.NotifyAccountReachoutTimelock, error) {
	data, err := gows.int.SendMexIQ(ctx, queryFetchAccountReachoutTimelock, Map{})
	if err != nil {
		return nil, err
	}
	if data == nil {
		return &events.NotifyAccountReachoutTimelock{}, nil
	}
	var respData respFetchAccountReachoutTimelock
	err = json.Unmarshal(data, &respData)
	if err != nil {
		return nil, err
	}
	if respData.Timelock == nil {
		return &events.NotifyAccountReachoutTimelock{}, nil
	}
	return respData.Timelock, nil
}
