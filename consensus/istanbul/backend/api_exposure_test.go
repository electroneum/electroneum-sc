package backend

import (
	"testing"

	"github.com/electroneum/electroneum-sc/common"
	"github.com/electroneum/electroneum-sc/node"
	"github.com/electroneum/electroneum-sc/rpc"
)

// istanbul_propose and istanbul_discard cast this validator's vote, so the
// istanbul namespace is only served over HTTP or WS when it's listed in
// --http.api / --ws.api, like admin, miner and personal. An empty list must
// not pick it up.
func TestAPIs_NotServedUnlessListed(t *testing.T) {
	chain, engine := newBlockChain(1)
	defer engine.Stop()

	for name, modules := range map[string][]string{
		"empty module list":    nil,
		"istanbul listed":      {"istanbul"},
		"other modules listed": {"eth"},
	} {
		t.Run(name, func(t *testing.T) {
			server := rpc.NewServer()
			defer server.Stop()
			if err := node.RegisterApis(engine.APIs(chain), modules, server, false); err != nil {
				t.Fatal(err)
			}
			client := rpc.DialInProc(server)
			defer client.Close()

			err := client.Call(nil, "istanbul_propose", common.Address{0x01}, true)
			served := len(modules) == 1 && modules[0] == "istanbul"
			if served && err != nil {
				t.Fatalf("istanbul_propose not served when listed: %v", err)
			}
			if !served && err == nil {
				t.Fatal("istanbul_propose served without being listed")
			}
		})
	}
}
