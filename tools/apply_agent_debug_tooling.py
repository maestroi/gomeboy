from pathlib import Path

p = Path("pkg/gomeboy/gomeboy.go")
s = p.read_text()

repls = [
    (
        "type Emulator struct {\n\tgb *gameboy.GameBoy\n}\n",
        "type Emulator struct {\n\tgb *gameboy.GameBoy\n\n\t// Optional agent/debug tooling. Both are inert unless explicitly enabled.\n\tflight         *FlightRecorder\n\tinputRecording bool\n\tinputLog       []InputEvent\n}\n",
    ),
    (
        "func (e *Emulator) Press(b Button) {\n\te.gb.Bus.Press(buttonMap[b])\n}\n",
        "func (e *Emulator) Press(b Button) {\n\te.gb.Bus.Press(buttonMap[b])\n\te.recordInputEvent(b, true)\n}\n",
    ),
    (
        "func (e *Emulator) Release(b Button) {\n\te.gb.Bus.Release(buttonMap[b])\n}\n",
        "func (e *Emulator) Release(b Button) {\n\te.gb.Bus.Release(buttonMap[b])\n\te.recordInputEvent(b, false)\n}\n",
    ),
    (
        "func (e *Emulator) StepFrame() {\n\te.gb.Step()\n}\n",
        "func (e *Emulator) StepFrame() {\n\te.gb.Step()\n\te.recordFlightFrame()\n}\n",
    ),
    (
        "func (e *Emulator) StepFrames(n int) {\n\te.gb.StepFrames(n)\n}\n",
        "func (e *Emulator) StepFrames(n int) {\n\tif e.flight != nil && e.flight.needsFrameSampling() {\n\t\tfor i := 0; i < n; i++ {\n\t\t\te.StepFrame()\n\t\t}\n\t\treturn\n\t}\n\te.gb.StepFrames(n)\n}\n",
    ),
]

for old, new in repls:
    if old not in s:
        raise SystemExit(f"expected snippet not found:\n{old}")
    s = s.replace(old, new, 1)

p.write_text(s)
