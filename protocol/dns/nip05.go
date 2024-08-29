package dns

import (
	"encoding/json"
	"fmt"
	"net/http"
	. "nostr.mleku.dev"
	"nostr.mleku.dev/codec/bech32encoding/pointers"
	"nostr.mleku.dev/crypto/keys"
	"regexp"
	"strings"
)

var Nip05Regex = regexp.MustCompile(`^(?:([\w.+-]+)@)?([\w_-]+(\.[\w_-]+)+)$`)

type WellKnownResponse struct {
	Names  map[S]S   `json:"names"`
	Relays map[S][]S `json:"relays,omitempty"`
	NIP46  map[S][]S `json:"nip46,omitempty"`
}

func IsValidIdentifier(input S) bool {
	return Nip05Regex.MatchString(input)
}

func ParseIdentifier(account S) (name, domain S, err error) {
	res := Nip05Regex.FindStringSubmatch(account)
	if len(res) == 0 {
		return "", "", Errorf.E("invalid identifier")
	}
	if res[1] == "" {
		res[1] = "_"
	}
	return res[1], res[2], nil
}

func QueryIdentifier(c Ctx, account S) (prf *pointers.Profile, err E) {
	var result WellKnownResponse
	var name S
	if result, name, err = Fetch(c, account); Chk.E(err) {
		return
	}
	pubkey, ok := result.Names[name]
	if !ok {
		err = Errorf.E("no entry for name '%s'", name)
		return
	}
	if !keys.IsValidPublicKey(pubkey) {
		return nil, Errorf.E("got an invalid public key '%s'", pubkey)
	}
	var pkb B
	if pkb, err = keys.HexPubkeyToBytes(pubkey); Chk.E(err) {
		return
	}
	relays, _ := result.Relays[pubkey]
	return &pointers.Profile{
		PublicKey: pkb,
		Relays:    StringSliceToByteSlice(relays),
	}, nil
}

func Fetch(c Ctx, account S) (resp WellKnownResponse, name S, err error) {
	var domain S
	if name, domain, err = ParseIdentifier(account); Chk.E(err) {
		err = Errorf.E("failed to parse '%s': %w", account, err)
		return
	}
	var req *http.Request
	if req, err = http.NewRequestWithContext(c, "GET",
		fmt.Sprintf("https://%s/.well-known/nostr.json?name=%s", domain, name), nil); Chk.E(err) {

		return resp, name, Errorf.E("failed to create a request: %w", err)
	}
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error { return http.ErrUseLastResponse },
	}
	var res *http.Response
	if res, err = client.Do(req); Chk.E(err) {
		err = Errorf.E("request failed: %w", err)
		return
	}
	defer res.Body.Close()
	var result WellKnownResponse
	if err = json.NewDecoder(res.Body).Decode(&result); Chk.E(err) {
		err = Errorf.E("failed to decode json response: %w", err)
		return
	}
	return
}

func NormalizeIdentifier(account S) S {
	if strings.HasPrefix(account, "_@") {
		return account[2:]
	}
	return account
}
