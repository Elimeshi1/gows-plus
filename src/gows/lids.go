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
	for _, child := range list.GetChildren() {
		if child.Tag != "user" {
			continue
		}
		ag := child.AttrGetter()
		jid := ag.OptionalJIDOrEmpty("jid")
		pnJid := ag.OptionalJIDOrEmpty("pn_jid")
		// The phone number may come either in the pn_jid attribute (lid addressing) or as the canonical jid
		var pn = types.EmptyJID
		if pnJid.Server == types.DefaultUserServer {
			pn = pnJid
		} else if jid.Server == types.DefaultUserServer {
			pn = jid
		}
		if pn.IsEmpty() {
			continue
		}
		gows.StoreLIDPNMapping(ctx, lid, pn)
		return pn, nil
	}
	return types.EmptyJID, nil
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
