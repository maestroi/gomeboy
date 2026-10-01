package power

// State captures the readable BIOS system-control state.
type State struct{ POSTFlag byte }

func (p *Controller) Snapshot() State { return State{POSTFlag: p.postFlag} }
func (p *Controller) Restore(s State) { p.postFlag = s.POSTFlag & 1 }
