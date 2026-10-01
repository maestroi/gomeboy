package interrupt

// State captures IE/IF/IME without scheduler hooks.
type State struct {
	IE    uint16
	Flags uint16
	IME   bool
}

func (c *Controller) Snapshot() State { return State{IE: c.ie, Flags: c.flags, IME: c.ime} }

func (c *Controller) Restore(s State) {
	c.ie = s.IE & validMask
	c.flags = s.Flags & validMask
	c.ime = s.IME
	if !c.hooks.DeferLine && c.sink != nil {
		c.sink.SetIRQLine(c.IRQAsserted())
	}
}
