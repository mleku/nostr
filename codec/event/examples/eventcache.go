package examples

import (
	_ "embed"

	. "nostr.mleku.dev"
)

// todo: re-encode this stuff as binary events with compression

//go:embed out.jsonl
var Cache B
