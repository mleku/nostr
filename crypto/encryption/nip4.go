package encryption

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"strings"

	"lukechampine.com/frand"
	"nostr.mleku.dev/crypto/p256k"
	"util.mleku.dev/hex"
)

// ComputeSharedSecret returns a shared secret key used to encrypt messages. The private and public keys should be hex
// encoded. Uses the Diffie-Hellman key exchange (ECDH) (RFC 4753).
func ComputeSharedSecret(pkh, skh S) (sharedSecret B, err E) {
	var skb, pkb B
	if skb, err = hex.Dec(skh); chk.E(err) {
		return
	}
	if pkb, err = hex.Dec(pkh); chk.E(err) {
		return
	}
	signer := new(p256k.Signer)
	if err = signer.InitSec(skb); chk.E(err) {
		return
	}
	if sharedSecret, err = signer.ECDH(pkb); chk.E(err) {
		return
	}
	return
}

// EncryptNip4 encrypts message with key using aes-256-cbc. key should be the shared secret generated by
// ComputeSharedSecret.
//
// Returns: base64(encrypted_bytes) + "?iv=" + base64(initialization_vector).
//
// Deprecated: upgrade to using Decrypt with the NIP-44 algorithm.
func EncryptNip4(msg S, key B) (ct B, err E) {
	// block size is 16 bytes
	iv := make(B, 16)
	if _, err = frand.Read(iv); chk.E(err) {
		err = errorf.E("error creating initialization vector: %w", err)
		return
	}
	// automatically picks aes-256 based on key length (32 bytes)
	var block cipher.Block
	if block, err = aes.NewCipher(key); chk.E(err) {
		err = errorf.E("error creating block cipher: %w", err)
		return
	}
	mode := cipher.NewCBCEncrypter(block, iv)
	plaintext := B(msg)
	// add padding
	base := len(plaintext)
	// this will be a number between 1 and 16 (inclusive), never 0
	bs := block.BlockSize()
	padding := bs - base%bs
	// encode the padding in all the padding bytes themselves
	padText := bytes.Repeat(B{byte(padding)}, padding)
	paddedMsgBytes := append(plaintext, padText...)
	ciphertext := make(B, len(paddedMsgBytes))
	mode.CryptBlocks(ciphertext, paddedMsgBytes)
	return B(base64.StdEncoding.EncodeToString(ciphertext) + "?iv=" +
		base64.StdEncoding.EncodeToString(iv)), nil
}

// DecryptNip4 decrypts a content string using the shared secret key. The inverse operation to message ->
// EncryptNip4(message, key).
//
// Deprecated: upgrade to using Decrypt with the NIP-44 algorithm.
func DecryptNip4(content S, key B) (msg B, err E) {
	parts := strings.Split(content, "?iv=")
	if len(parts) < 2 {
		return nil, errorf.E(
			"error parsing encrypted message: no initialization vector")
	}
	var ciphertext B
	if ciphertext, err = base64.StdEncoding.DecodeString(parts[0]); chk.E(err) {
		err = errorf.E("error decoding ciphertext from base64: %w", err)
		return
	}
	var iv B
	if iv, err = base64.StdEncoding.DecodeString(parts[1]); chk.E(err) {
		err = errorf.E("error decoding iv from base64: %w", err)
		return
	}
	var block cipher.Block
	if block, err = aes.NewCipher(key); chk.E(err) {
		err = errorf.E("error creating block cipher: %w", err)
		return
	}
	mode := cipher.NewCBCDecrypter(block, iv)
	msg = make(B, len(ciphertext))
	mode.CryptBlocks(msg, ciphertext)
	// remove padding
	var (
		plaintextLen = len(msg)
	)
	if plaintextLen > 0 {
		// the padding amount is encoded in the padding bytes themselves
		padding := int(msg[plaintextLen-1])
		if padding > plaintextLen {
			err = errorf.E("invalid padding amount: %d", padding)
			return
		}
		msg = msg[0 : plaintextLen-padding]
	}
	return msg, nil
}
