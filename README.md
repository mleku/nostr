# nostr

nostr codec and protocol libraries written for performance and simplicity

codec dispenses with all strings, uses binary in fields that strictly 
represent binary (id, pubkey, sig, a and e tags, and likewise in the filters)

single common marshal/unmarshal interface that allows you to provide your 
own buffers so you can manage recycling them... unmarshalled data almost all 
uses the buffer fields directly from the buffers they are provided on, 
avoiding memory allocation

uses the bitcoin-core secp256k1 signature library for 4x improvement in 
signing and 2x improvement in verification

provides the fastest existing binary encoder for events for use with 
disk/database storage, combines with the binary data in the runtime data 
structures for face-melting performance

concise and clear APIs for everything including relay websockets, 
authentication, relay information, signing and verification and ECDH shared 
secret generation, and all elements of filter and events

## notes about secp256k1 library

see [p256k1 docs](crypto/p256k/README.md) for building with the 
`bitcoin-core/secp256k1` library interfaced with CGO (it is about 2x faster 
at verification and 4x faster at signing) but if you don't want to use CGO 
or can't, set the build tag `btcec` to disable the `secp256k1` CGO binding 
interface. The CGO version is default because it is so much better, deal 
with it.

## running tests and benches

use the script at [scripts/runtests.sh](scripts/runtests.sh) to get a full suite of tests and 
benchmarks including memory utilization information
