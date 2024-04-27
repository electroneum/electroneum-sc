# Electroneum Smart Chain

Electroneum Smart Chain implementation based on go-ethereum. 

Electroneum Smart Chain is EVM-compatible, supports all the existing Ethereum tooling, provides nearly instant transaction verification and 1-block finality with a modified version of the Istanbul Byzantine Fault Tolerance (IBFT) consensus protocol.

## Key Features

### IBFT Consensus Protocol

Electroneum Smart Chain implements a modified version of the standard IBFT proof of authority consensus protocol, making it the perfect consensus algorithm for public blockchains with a consortium of publicly-known validators participating in the block creation. Existing validators propose and vote to add or remove validators through our on-chain voting system.

This state-of-the-art consensus protocol features:

- **Immediate Finality:** blocks are final, meaning there are no forks or concurrent alt-chains, and valid blocks must be in the main chain
- **Nearly Instant Confirmations:** blocks are created every 5 seconds
- **Dynamic Validator Set:** validators can be added or removed from the network by an on-chain voting mechanism
- **Optimal Byzantine Resilience:** the protocol can withstand up to `(n-1)/3` Byzantine validators, where 
`n` is the number of validators

### EVM-Compatible

Electroneum Smart Chain supports all the existing Ethereum tooling, smart contracts, decentralized applications and regular applications based on the Ethereum JSON RPC, such as MetaMask.

### Cross-chain Bridge

Electroneum Smart Chain supports cross-chain transfers between our legacy Electroneum Blockchain and the Smart Chain. All users, exchanges and other service providers can seamlessly transfer their funds over to the Electroneum Smart Chain, free of charge.


## Building the source

For prerequisites and detailed build instructions please read the [Installation Instructions](https://github.com/electroneum/electroneum-sc/wiki/Install-and-Build).

Building `etn-sc` requires both a Go (version 1.19 or later) and a C compiler. You can install
them using your favourite package manager. Once the dependencies are installed, run

```shell
make etn-sc
```

or, to build the full suite of utilities:

```shell
make all
```

## Executables

The electroneum-sc project comes with several wrappers/executables found in the `cmd`
directory.

|    Command    | Description                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                          |
| :-----------: | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
|  **`etn-sc`**   | Our main Electroneum Smart Chain CLI client. It is the entry point into the Electroneum-SC network (main-, test- or private net), capable of running as a full node (default), archive node (retaining all historical state) or a light node (retrieving data live). It can be used by other processes as a gateway into the Electroneum-SC network via JSON RPC endpoints exposed on top of HTTP, WebSocket and/or IPC transports. `etn-sc --help` and the [CLI page](https://geth.ethereum.org/docs/interface/command-line-options) for command line options.          |
|   `clef`    | Stand-alone signing tool, which can be used as a backend signer for `etn-sc`.  |
|   `devp2p`    | Utilities to interact with nodes on the networking layer, without running a full blockchain. |
|   `abigen`    | Source code generator to convert Electroneum contract definitions into easy to use, compile-time type-safe Go packages. It operates on plain [Ethereum contract ABIs](https://docs.soliditylang.org/en/develop/abi-spec.html) with expanded functionality if the contract bytecode is also available. However, it also accepts Solidity source files, making development much more streamlined. Please see our [Native DApps](https://geth.ethereum.org/docs/dapp/native-bindings) page for details. |
|  `bootnode`   | Stripped down version of our Electroneum-SC client implementation that only takes part in the network node discovery protocol, but does not run any of the higher level application protocols. It can be used as a lightweight bootstrap node to aid in finding peers in private networks.                                                                                                                                                                                                                                                                 |
|     `evm`     | Developer utility version of the EVM (Ethereum Virtual Machine) that is capable of running bytecode snippets within a configurable environment and execution mode. Its purpose is to allow isolated, fine-grained debugging of EVM opcodes (e.g. `evm --code 60ff60ff --debug run`).                                                                                                                                                                                                                                                                     |
|   `rlpdump`   | Developer utility tool to convert binary RLP ([Recursive Length Prefix](https://eth.wiki/en/fundamentals/rlp)) dumps (data encoding used by the Ethereum protocol both network as well as consensus wise) to user-friendlier hierarchical representation (e.g. `rlpdump --hex CE0183FFFFFFC4C304050583616263`).                                                                                                                                                                                                                                 |
|   `puppeth`   | a CLI wizard that aids in creating a new Electroneum-SC network.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                           |

## Running `etn-sc`

Going through all the possible command line flags is out of scope here (please consult our
[CLI Wiki page](https://geth.ethereum.org/docs/interface/command-line-options)),
but we've enumerated a few common parameter combos to get you up to speed quickly
on how you can run your own `etn-sc` instance.

### Hardware Requirements

Minimum:

* CPU with 2+ cores
* 4GB RAM
* 8 MBit/sec download Internet service

Recommended:

* Fast CPU with 4+ cores
* 16GB+ RAM
* High Performance SSD
* 25+ MBit/sec download Internet service

### Full node on the main Electroneum Smart Chain network

By far the most common scenario is people wanting to simply interact with the Electroneum Smart Chain
network: create accounts; transfer funds; deploy and interact with contracts. For this
particular use-case the user doesn't care about years-old historical data, so we can
sync quickly to the current state of the network. To do so:

```shell
$ etn-sc console
```

This command will:
 * Start `etn-sc` in snap sync mode (default, can be changed with the `--syncmode` flag),
   causing it to download more data in exchange for avoiding processing the entire history
   of the Electroneum Smart Chain network, which is very CPU intensive.
 * Start up `etn-sc`'s built-in interactive [JavaScript console](https://geth.ethereum.org/docs/interface/javascript-console),
   (via the trailing `console` subcommand) through which you can interact using [`web3` methods](https://github.com/ChainSafe/web3.js/blob/0.20.7/DOCUMENTATION.md) 
   (note: the `web3` version bundled within `etn-sc` is very old, and not up to date with official docs),
   as well as `etn-sc`'s own [management APIs](https://geth.ethereum.org/docs/rpc/server).
   This tool is optional and if you leave it out you can always attach to an already running
   `etn-sc` instance with `etn-sc attach`.
 * Write blockchain data to the default data directory: default data directory (`~/.electroneum-sc` on linux, `C:\Users\<username>\AppData\Roaming\Electroneum-sc` on Windows and `~/Library/Electroneum-sc` on Darwin) 

### Full node on the test network

Transitioning towards developers, if you'd like to play around with creating Electroneum
contracts, you almost certainly would like to do that without any real cryptocurrency involved until
you get the hang of the entire system. In other words, instead of attaching to the main
network, you want to join the **test** network with your node, which is fully equivalent to
the main network, but with play-ETN only.

```shell
$ etn-sc --testnet console
```

The `console` subcommand has the exact same meaning as above and they are equally
useful on the testnet too. Please, see above for their explanations if you've skipped here.

Specifying the `--testnet` flag, however, will reconfigure your `etn-sc` instance a bit:

 * Instead of connecting the main Electroneum Smart Chain network, the client will connect to the test network, which uses different P2P bootnodes, different network IDs and genesis
   states.
 * Instead of using the default data directory (`~/.electroneum-sc` on Linux for example), `etn-sc`
   will nest itself one level deeper into a `testnet` subfolder (`~/.electroneum-sc/testnet` on
   Linux). Note, on OSX and Linux this also means that attaching to a running testnet node
   requires the use of a custom endpoint since `etn-sc attach` will try to attach to a
   production node endpoint by default, e.g.,
   `etn-sc attach <datadir>/testnet/etn-sc.ipc`. Windows users are not affected by
   this.

*Note: Although there are some internal protective measures to prevent transactions from
crossing over between the main network and test network, you should make sure to always
use separate accounts for play-cryptocurrency and real-cryptocurrency. Unless you manually move
accounts, `etn-sc` will by default correctly separate the two networks and will not make any
accounts available between them.*

### Configuration

As an alternative to passing the numerous flags to the `etn-sc` binary, you can also pass a
configuration file via:

```shell
$ etn-sc --config /path/to/your_config.toml
```

To get an idea how the file should look like you can use the `dumpconfig` subcommand to
export your existing configuration:

```shell
$ etn-sc --your-favourite-flags dumpconfig
```
### Docker quick start

One of the quickest ways to get Electroneum Smart Chain up and running on your machine is by using
Docker:

```shell
docker run -d --name etn-sc-node -v /Users/alice/electroneum:/root \
           -p 8545:8545 -p 30303:30303 \
           electroneum/client-go
```

This will start `etn-sc` in snap-sync mode with a DB memory allowance of 1GB just as the
above command does.  It will also create a persistent volume in your home directory for
saving your blockchain as well as map the default ports. There is also an `alpine` tag
available for a slim version of the image.

Do not forget `--http.addr 0.0.0.0`, if you want to access RPC from other containers
and/or hosts. By default, `etn-sc` binds to the local interface and RPC endpoints are not
accessible from the outside.

### Programmatically interfacing `etn-sc` nodes

As a developer, sooner rather than later you'll want to start interacting with `etn-sc` and the
Electroneum Smart Chain network via your own programs and not manually through the console. To aid
this, `etn-sc` has built-in support for a JSON-RPC based APIs ([standard APIs](https://eth.wiki/json-rpc/API)
and [`etn-sc` specific APIs](https://geth.ethereum.org/docs/rpc/server)).
These can be exposed via HTTP, WebSockets and IPC (UNIX sockets on UNIX based
platforms, and named pipes on Windows).

The IPC interface is enabled by default and exposes all the APIs supported by `etn-sc`,
whereas the HTTP and WS interfaces need to manually be enabled and only expose a
subset of APIs due to security reasons. These can be turned on/off and configured as
you'd expect.

HTTP based JSON-RPC API options:

  * `--http` Enable the HTTP-RPC server
  * `--http.addr` HTTP-RPC server listening interface (default: `localhost`)
  * `--http.port` HTTP-RPC server listening port (default: `8545`)
  * `--http.api` API's offered over the HTTP-RPC interface (default: `eth,net,web3`)
  * `--http.corsdomain` Comma separated list of domains from which to accept cross origin requests (browser enforced)
  * `--ws` Enable the WS-RPC server
  * `--ws.addr` WS-RPC server listening interface (default: `localhost`)
  * `--ws.port` WS-RPC server listening port (default: `8546`)
  * `--ws.api` API's offered over the WS-RPC interface (default: `eth,net,web3`)
  * `--ws.origins` Origins from which to accept websockets requests
  * `--ipcdisable` Disable the IPC-RPC server
  * `--ipcapi` API's offered over the IPC-RPC interface (default: `admin,debug,eth,miner,net,personal,shh,txpool,web3`)
  * `--ipcpath` Filename for IPC socket/pipe within the datadir (explicit paths escape it)

You'll need to use your own programming environments' capabilities (libraries, tools, etc) to
connect via HTTP, WS or IPC to a `etn-sc` node configured with the above flags and you'll
need to speak [JSON-RPC](https://www.jsonrpc.org/specification) on all transports. You
can reuse the same connection for multiple requests!

**Note: Please understand the security implications of opening up an HTTP/WS based
transport before doing so! Hackers on the internet are actively trying to subvert
Electroneum nodes with exposed APIs! Further, all browser tabs can access locally
running web servers, so malicious web pages could try to subvert locally available
APIs!**

## Contribution

Thank you for considering to help out with the source code! We welcome contributions
from anyone on the internet, and are grateful for even the smallest of fixes!

If you'd like to contribute to electroneum-sc, please fork, fix, commit and send a pull request
for the maintainers to review and merge into the main code base. If you wish to submit
more complex changes though, please check up with the core devs first on [our Discord Server](https://discord.gg/mBzrW9SvkJ)
to ensure those changes are in line with the general philosophy of the project and/or get
some early feedback which can make both your efforts much lighter as well as our review
and merge procedures quick and simple.

Please make sure your contributions adhere to our coding guidelines:

 * Code must adhere to the official Go [formatting](https://golang.org/doc/effective_go.html#formatting)
   guidelines (i.e. uses [gofmt](https://golang.org/cmd/gofmt/)).
 * Code must be documented adhering to the official Go [commentary](https://golang.org/doc/effective_go.html#commentary)
   guidelines.
 * Pull requests need to be based on and opened against the `master` branch.
 * Commit messages should be prefixed with the package(s) they modify.
   * E.g. "etn, rpc: make trace configs optional"


## License

The electroneum-sc and go-ethereum library (i.e. all code outside of the `cmd` directory) is licensed under the
[GNU Lesser General Public License v3.0](https://www.gnu.org/licenses/lgpl-3.0.en.html),
also included in our repository in the `COPYING.LESSER` file.

The electroneum-sc and go-ethereum binaries (i.e. all code inside of the `cmd` directory) is licensed under the
[GNU General Public License v3.0](https://www.gnu.org/licenses/gpl-3.0.en.html), also
included in our repository in the `COPYING` file.
