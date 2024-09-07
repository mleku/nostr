package event

import (
	sch "ec.mleku.dev/v2/schnorr"
	k1 "ec.mleku.dev/v2/secp256k1"
	. "nostr.mleku.dev"
	"nostr.mleku.dev/crypto"
	"nostr.mleku.dev/crypto/p256k"
)

// Sign the event using the nostr.Signer. Uses github.com/bitcoin-core/secp256k1 if available for much faster
// signatures.
func (ev *T) Sign(keys crypto.Signer) (err error) {
	ev.ID = ev.GetIDBytes()
	if ev.Sig, err = keys.Sign(ev.ID); Chk.E(err) {
		return
	}
	ev.PubKey = keys.Pub()
	return
}

// Verify an event is signed by the pubkey it contains. Uses
// github.com/bitcoin-core/secp256k1 if available for faster verification.
func (ev *T) Verify() (valid bool, err error) {
	keys := p256k.Signer{}
	if err = keys.InitPub(ev.PubKey); Chk.E(err) {
		return
	}
	if valid, err = keys.Verify(ev.ID, ev.Sig); Chk.T(err) {
		// check that this isn't because of a bogus ID
		id := ev.GetIDBytes()
		if !Equals(id, ev.ID) {
			Log.E.Ln("event ID incorrect")
			ev.ID = id
			err = nil
			if valid, err = keys.Verify(ev.ID, ev.Sig); Chk.E(err) {
				return
			}
			err = Errorf.W("event ID incorrect but signature is valid on correct ID")
		}
		return
	}
	return
}

// SignWithSecKey signs an event with a given *secp256xk1.SecretKey.
//
// Deprecated: use Sign and nostr.Signer and p256k.Signer / p256k.BTCECSigner
// implementations.
func (ev *T) SignWithSecKey(sk *k1.SecretKey,
	so ...sch.SignOption) (err error) {

	// sign the event.
	var sig *sch.Signature
	ev.ID = ev.GetIDBytes()
	if sig, err = sch.Sign(sk, ev.ID, so...); Chk.D(err) {
		return
	}
	// we know secret key is good so we can generate the public key.
	ev.PubKey = sch.SerializePubKey(sk.PubKey())
	ev.Sig = sig.Serialize()
	return
}

// CheckSignature returns whether an event signature is authentic and matches
// the event ID and Pubkey.
//
// Deprecated: use Verify
func (ev *T) CheckSignature() (valid bool, err error) {
	// parse pubkey bytes.
	var pk *k1.PublicKey
	if pk, err = sch.ParsePubKey(ev.PubKey); Chk.D(err) {
		err = Errorf.E("event has invalid pubkey '%0x': %v", ev.PubKey, err)
		return
	}
	// parse signature bytes.
	var sig *sch.Signature
	if sig, err = sch.ParseSignature(ev.Sig); Chk.D(err) {
		err = Errorf.E("failed to parse signature:\n%d %s\n%v", len(ev.Sig),
			ev.Sig, err)
		return
	}
	// check signature.
	valid = sig.Verify(ev.GetIDBytes(), pk)
	return
}
