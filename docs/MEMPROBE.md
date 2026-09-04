# Deterministic memory probing

`pkg/memprobe` is a game-agnostic layer above GomeBoy's introspection API for controlled reverse-engineering experiments.

It is designed for agents and scripts that need causal evidence about memory without handing a model a raw 64 KiB dump or teaching the emulator game-specific concepts.

## Core experiment

Every call to `memprobe.Run` uses one shared baseline:

```text
capture checkpoint S
capture observed memory regions

restore S -> action A -> observe -> diff
restore S -> action B -> observe -> diff
restore S -> action C -> observe -> diff

restore S
```

Because every action begins from the same emulation state, an external agent can compare outcomes directly. A no-input control action helps distinguish ordinary time-driven state changes from changes correlated with a button input.

The runner restores the emulation core checkpoint before returning, including error paths. Optional `gomeboy.Emulator` diagnostic histories such as an attached flight recorder or active input-recording log are outside the checkpoint and are not rewound by `memprobe`.

## Go API

```go
regions := []memprobe.Region{
    {Name: "wram", Start: 0xC000, Length: 0x2000},
    {Name: "hram", Start: 0xFF80, Length: 0x7F},
}

actions := []memprobe.Action{
    memprobe.Wait("control", 8),
    memprobe.Tap("right", gomeboy.ButtonRight, 1, 7),
    memprobe.Tap("left", gomeboy.ButtonLeft, 1, 7),
}

results, err := memprobe.Run(emu, regions, actions)
if err != nil {
    log.Fatal(err)
}

for _, result := range results {
    for _, change := range result.Changes {
        fmt.Printf("%s %04X: %02X -> %02X (%+d)\n",
            result.Action,
            change.Address,
            change.Before,
            change.After,
            change.Delta,
        )
    }
}
```

A phase can contain multiple input transitions followed by an exact number of display frames. Empty-transition phases are waits, so callers can build input sequences more complex than a simple tap without adding game-specific logic to the package.

Memory regions must not wrap around `0xFFFF`. Overlapping regions are allowed; if they overlap, a changed address is reported under each relevant region label.

## CLI

`cmd/memprobe` exposes the same primitive as JSON for external agents and shell tooling.

Build it with:

```bash
go build ./cmd/memprobe
```

Probe the normal GB/GBC work RAM and high RAM from the state reached after loading a ROM:

```bash
./memprobe -rom path/to/game.gb
```

By default the command executes equal-length experiments for:

```text
control, up, down, left, right, a, b, start, select
```

The default tap holds a button for one frame and settles for seven frames. The control action advances the same total eight frames with no input.

Useful flags:

```text
-rom       ROM path (required)
-state     optional raw state produced by Emulator.SaveState
-regions   comma-separated name:START-END ranges in hexadecimal
-actions   comma-separated built-in actions
-warmup    frames to advance before capturing the experiment baseline
-hold      frames to hold each tapped button
-settle    frames to advance after button release
-compact   emit compact JSON
```

For example:

```bash
./memprobe \
  -rom path/to/game.gbc \
  -state interesting-state.bin \
  -regions 'wram:C000-DFFF,hram:FF80-FFFE' \
  -actions 'control,left,right' \
  -hold 1 \
  -settle 7
```

The JSON includes the ROM SHA-256, cartridge title, hardware model, common baseline frame/cycle, and per-action changes. Addresses are emitted both numerically and in hexadecimal for convenient machine and human consumption.

## How an agent can use the evidence

The first useful inference layer should stay outside GomeBoy. It can consume repeated `memprobe` results and score hypotheses such as:

```text
RIGHT:  C109 07 -> 08
RIGHT:  C109 08 -> 09
LEFT:   C109 09 -> 08
control C109 unchanged

hypothesis: C109 behaves like a horizontal position/counter
confidence: high
```

The important distinction is that `memprobe` produces deterministic observations; it does not assign semantic names. A downstream agent can repeat experiments, vary action duration, return to a checkpoint, or move to another prepared state to test its own hypotheses.

## Deliberate non-goals

This layer does not currently provide:

- game-specific addresses or schemas
- automatic semantic naming
- an LLM or OpenAI API dependency
- screenshots or vision
- arbitrary RAM mutation
- cycle-accurate attempted-write tracing
- direct inactive-bank CGB WRAM/VRAM inspection

Those can be added independently without coupling the emulator core to one game or one agent implementation.
