package ppu

import "github.com/maestroi/gomeboy/internal/types"

func (p *PPU) setVisibleLY(v byte) {
	p.b.Set(types.LY, v)
}
