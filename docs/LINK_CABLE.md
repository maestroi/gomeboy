# Network link cable

GomeBoy exposes the Game Boy serial port as a generic network link. The core
contains no game-specific trading or battle logic.

## Topologies

### Direct peer-to-peer

Two emulator processes can connect over a normal TCP connection. One side
dials with `gomeboy.DialDirectLink`; the listening side accepts a `net.Conn`
and wraps it with `gomeboy.NewDirectLink`.

### Brokered sessions

`cmd/gomeboy-link-broker` accepts multiple concurrent two-peer sessions:

```sh
go run ./cmd/gomeboy-link-broker -listen :8765
```

An emulator joins a session with:

```go
networkLink, err := emu.ConnectBrokerLink(
    "link-broker:8765",
    "run-42",
    "red",
    "emulator",
    map[string]string{"game": "pokemon-red"},
    10*time.Second,
)
```

The port only needs to be reachable on the Docker network; it does not need to
be published to the host.

## Virtual peers

The public `pkg/link` package is the wire contract. A process that does not run
GomeBoy can join a broker session with role `virtual`, consume `clock`
messages, and answer with `clock_reply` messages. This allows higher-level
projects to emulate a cartridge-specific peer without adding cartridge or game
knowledge to GomeBoy.

For example, PokéPilot can implement a Generation-I Pokémon trader that
constructs valid trainer/party traffic while GomeBoy continues to emulate only
the serial hardware.

## Metadata

`hello` and `metadata` messages contain optional `map[string]string` metadata.
The broker treats it as opaque and forwards it to the other endpoint. Intended
uses include run IDs, game identifiers, ROM hashes, agent state, or virtual-peer
capabilities. Metadata never changes serial timing.

## Serial timing

The network protocol transports individual clocked bits rather than writing SB
or SC remotely.

When the local Game Boy supplies the internal clock, the serial scheduler sends
one `clock` message for each hardware bit transfer and waits for the peer's
`clock_reply`.

When the remote endpoint supplies the clock, the network reader queues the
incoming pulse. The emulator consumes that queue using a scheduler event and
updates SB/SC on the emulator thread. Network goroutines therefore never mutate
Game Boy bus state directly.

If the peer has requested an external-clock transfer but no pulse is ready yet,
the serial controller backs off unsuccessful polls (32 ticks, doubling up to
4096). That keeps a halted CPU from spending every scheduler skip on the serial
event while a slow or mid-negotiation network peer is silent. The pending
transfer is not cancelled.

A completed eight-bit transfer continues to use the existing serial controller
to clear SC bit 7 and raise the serial interrupt.

## Protocol

Messages are newline-delimited JSON and include a protocol version. The main
message types are:

- `hello`: join a broker session
- `ready`: broker accepted the endpoint
- `clock`: one serial clock edge and outgoing bit
- `clock_reply`: peer bit sampled on that edge
- `metadata`: optional out-of-band session data
- `error`: transport/session error

The broker pairs exactly two endpoints in each logical session. An endpoint may
be another emulator or any synthetic peer implementing `pkg/link`.

## Scope

GomeBoy intentionally does not know about Pokémon species, trades, Pokédex
rules, Mew, version exclusives, or trade evolutions. Those semantics belong in
the downstream virtual peer/orchestrator. This keeps the same link transport
usable by other Game Boy games and test fixtures.
