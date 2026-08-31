package gows

import (
	"context"

	waBinary "go.mau.fi/whatsmeow/binary"
	"go.mau.fi/whatsmeow/types"
)

// ResolvePNByLid resolves the phone number behind a LID - first from the local store,
// then, if unknown, via a server usync query. Returns an empty JID when the mapping is unknown.
func (gows *GoWS) ResolvePNByLid(ctx context.Context, lid types.JID) (types.JID, error) {
	if gows.Client == nil || gows.Store == nil || gows.Store.LIDs == nil {
		return types.EmptyJID, nil
	}
	pn, err := gows.Store.LIDs.GetPNForLID(ctx, lid)
	if err != nil {
		return types.EmptyJID, err
	}
	if !pn.IsEmpty() {
		return pn, nil
	}

	pn, err = gows.resolvePNByLidFromServer(ctx, lid)
	if err != nil {
		// Best effort - callers treat an empty JID as "unknown", do not fail the whole call
		gows.Log.Warnf("Failed to resolve PN for %v from server: %v", lid, err)
		return types.EmptyJID, nil
	}
	return pn, nil
}

// resolvePNByLidFromServer asks the server for the phone number behind a LID using the same
// usync "contact" query with addressing_mode=lid that IsOnWhatsApp uses (it returns pn_jid).
// The learned mapping is saved to the store, so the server is asked at most once per contact.
func (gows *GoWS) resolvePNByLidFromServer(ctx context.Context, lid types.JID) (types.JID, error) {
	if gows.int == nil {
		return types.EmptyJID, nil
	}
	list, err := gows.int.Usync(ctx, []types.JID{lid}, "query", "interactive", []waBinary.Node{
		{Tag: "contact", Attrs: waBinary.Attrs{"addressing_mode": "lid"}},
	})
	if err != nil {
		return types.EmptyJID, err
	}
	pn := pnFromUsyncResponse(lid, list)
	if pn.IsEmpty() {
		return types.EmptyJID, nil
	}
	if !gows.validLIDPNPair(lid, pn) {
		gows.Log.Warnf("Ignoring suspicious LID-PN resolution %v => %v", lid, pn)
		return types.EmptyJID, nil
	}
	gows.StoreLIDPNMapping(ctx, lid, pn)
	return pn, nil
}

// pnFromUsyncResponse extracts the phone number for the queried LID from the usync response.
// A pn_jid counts only when its user node is addressed to the queried LID; a bare phone jid counts
// only when it is the single user node - the answer to our single-user query.
func pnFromUsyncResponse(lid types.JID, list *waBinary.Node) types.JID {
	var users []waBinary.Node
	for _, child := range list.GetChildren() {
		if child.Tag == "user" {
			users = append(users, child)
		}
	}
	for _, user := range users {
		ag := user.AttrGetter()
		jid := ag.OptionalJIDOrEmpty("jid")
		pnJid := ag.OptionalJIDOrEmpty("pn_jid")
		if jid.Server == types.HiddenUserServer && jid.User == lid.User && pnJid.Server == types.DefaultUserServer {
			return pnJid
		}
		if len(users) == 1 && jid.Server == types.DefaultUserServer {
			return jid
		}
	}
	return types.EmptyJID
}

// validLIDPNPair rejects mapping a foreign LID to the own phone number - a peer can never be me.
func (gows *GoWS) validLIDPNPair(lid types.JID, pn types.JID) bool {
	ownPN := gows.Store.GetJID()
	ownLID := gows.Store.GetLID()
	if ownPN.IsEmpty() {
		return true
	}
	if pn.User == ownPN.User && lid.User != ownLID.User {
		return false
	}
	return true
}

// ResolveLidByPN resolves the LID for a phone number - first from the local store,
// then, if unknown, via GetUserInfo (which saves the lid mapping as a side effect).
func (gows *GoWS) ResolveLidByPN(ctx context.Context, pn types.JID) (types.JID, error) {
	if gows.Client == nil || gows.Store == nil || gows.Store.LIDs == nil {
		return types.EmptyJID, nil
	}
	lid, err := gows.Store.LIDs.GetLIDForPN(ctx, pn)
	if err != nil {
		return types.EmptyJID, err
	}
	if !lid.IsEmpty() {
		return lid, nil
	}

	_, err = gows.GetUserInfo(ctx, []types.JID{pn})
	if err != nil {
		gows.Log.Warnf("Failed to resolve LID for %v from server: %v", pn, err)
		return types.EmptyJID, nil
	}
	return gows.Store.LIDs.GetLIDForPN(ctx, pn)
}
