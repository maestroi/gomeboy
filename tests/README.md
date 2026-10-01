# Automated test results

This report is generated from the structured per-test result set used by CI and evaluated against `tests/regression-baseline.json`. PASS and XFAIL match the checked-in baseline; REGRESSION, XPASS, new, missing, or model-change entries require review.

![progress](https://progress-bar.xyz/92/?scale=100&title=passing%20231,%20failing%2021&width=500)

| Status | Count |
| --- | ---: |
| PASS | 231 |
| XFAIL | 21 |
| REGRESSION | 0 |
| XPASS | 0 |
| New tests | 0 |
| Missing tests | 0 |
| Model changes | 0 |

<hr/>
GomeBoy is automatically tested against the following test suites:

* **[Blargg's test roms](https://github.com/retrio/gb-test-roms)**  
  <sup>by [Shay Green (a.k.a. Blargg)](http://www.slack.net/~ant/) </sup>
* **[Bully](https://github.com/Hacktix/BullyGB)**, 
  **[scribbltests](https://github.com/Hacktix/scribbltests)** 
  and **[Strikethrough](https://github.com/Hacktix/strikethrough.gb)**  
  <sup>by [Hacktix](https://github.com/Hacktix) </sup>
* **[cgb-acid-hell](https://github.com/mattcurrie/cgb-acid-hell)**,
  **[cgb-acid2](https://github.com/mattcurrie/cgb-acid2)** and
  **[dmg-acid2](https://github.com/mattcurrie/dmg-acid2)**  
  <sup>by [Matt Currie](https://github.com/mattcurrie) </sup>
* **[(parts of) little-things-gb](https://github.com/pinobatch/little-things-gb)**  
  <sup>by [Damian Yerrick](https://github.com/pinobatch) </sup>
* **[Mooneye Test Suite](https://github.com/Gekkio/mooneye-test-suite)**  
  <sup>by [Joonas Javanainen](https://github.com/Gekkio) </sup>
* **[SameSuite](https://github.com/LIJI32/SameSuite)**  
  <sup>by [Lior Halphon](https://github.com/LIJI32) </sup>

Different test suites use different pass/fail criteria. Some may write output to the serial port such as
[Blargg's test roms](https://github.com/retrio/gb-test-roms), others may write to the CPU registers, such as 
[Mooneye Test Suite](https://github.com/Gekkio/mooneye-test-suite) and [SameSuite](https://github.com/LIJI32/SameSuite).
If the test suite does not provide a way to automatically determine a pass/fail criteria, then the emulator's output
is compared against a reference image from a known good emulator.
<hr/>


# Test Results
| Test Suite | Pass Rate | Tests Passed | Tests Failed | Tests Total |
| --- | --- | --- | --- | --- |
| acid2 | 75% | 3 | 1 | 4 |
| blarrg | 100% | 43 | 0 | 43 |
| bully | 100% | 2 | 0 | 2 |
| little-things-gb | 100% | 4 | 0 | 4 |
| mooneye | 99% | 113 | 1 | 114 |
| samesuite | 75% | 59 | 19 | 78 |
| scribbltests | 100% | 5 | 0 | 5 |
| strikethrough | 100% | 2 | 0 | 2 |

## Per-test results

### acid2

#### cgb-acid2
| Test | Expected | Actual | Status | Context |
| --- | --- | --- | --- | --- |
| cgb-acid-hell | fail | fail | xfail | Known failure imported from tests/README.md |
| cgb-acid2 | pass | pass | pass |  |

#### dmg-acid2
| Test | Expected | Actual | Status | Context |
| --- | --- | --- | --- | --- |
| dmg-acid2 (CGB) | pass | pass | pass |  |
| dmg-acid2 (DMG) | pass | pass | pass |  |

### blarrg

#### cgb_sound
| Test | Expected | Actual | Status | Context |
| --- | --- | --- | --- | --- |
| 01-registers | pass | pass | pass |  |
| 02-len ctr | pass | pass | pass |  |
| 03-trigger | pass | pass | pass |  |
| 04-sweep | pass | pass | pass |  |
| 05-sweep details | pass | pass | pass |  |
| 06-overflow on trigger | pass | pass | pass |  |
| 07-len sweep period sync | pass | pass | pass |  |
| 08-len ctr during power | pass | pass | pass |  |
| 09-wave read while on | pass | pass | pass |  |
| 10-wave trigger while on | pass | pass | pass |  |
| 11-regs after power | pass | pass | pass |  |
| 12-wave | pass | pass | pass |  |

#### cpu_instrs
| Test | Expected | Actual | Status | Context |
| --- | --- | --- | --- | --- |
| 01-special | pass | pass | pass |  |
| 02-interrupts | pass | pass | pass |  |
| 03-op sp,hl | pass | pass | pass |  |
| 04-op r,imm | pass | pass | pass |  |
| 05-op rp | pass | pass | pass |  |
| 06-ld r,r | pass | pass | pass |  |
| 07-jr,jp,call,ret,rst | pass | pass | pass |  |
| 08-misc instrs | pass | pass | pass |  |
| 09-op r,r | pass | pass | pass |  |
| 10-bit ops | pass | pass | pass |  |
| 11-op a,(hl) | pass | pass | pass |  |

#### dmg_sound
| Test | Expected | Actual | Status | Context |
| --- | --- | --- | --- | --- |
| 01-registers | pass | pass | pass |  |
| 02-len ctr | pass | pass | pass |  |
| 03-trigger | pass | pass | pass |  |
| 04-sweep | pass | pass | pass |  |
| 05-sweep details | pass | pass | pass |  |
| 06-overflow on trigger | pass | pass | pass |  |
| 07-len sweep period sync | pass | pass | pass |  |
| 08-len ctr during power | pass | pass | pass |  |
| 09-wave read while on | pass | pass | pass |  |
| 10-wave trigger while on | pass | pass | pass |  |
| 11-regs after power | pass | pass | pass |  |
| 12-wave write while on | pass | pass | pass |  |

#### halt_bug
| Test | Expected | Actual | Status | Context |
| --- | --- | --- | --- | --- |
| halt_bug (CGB) | pass | pass | pass |  |
| halt_bug (DMG) | pass | pass | pass |  |

#### instr_timing
| Test | Expected | Actual | Status | Context |
| --- | --- | --- | --- | --- |
| instr_timing | pass | pass | pass |  |

#### interrupt_time
| Test | Expected | Actual | Status | Context |
| --- | --- | --- | --- | --- |
| interrupt_time (CGB) | pass | pass | pass |  |
| interrupt_time (DMG) | pass | pass | pass |  |

#### mem_timing
| Test | Expected | Actual | Status | Context |
| --- | --- | --- | --- | --- |
| 01-read_timing | pass | pass | pass |  |
| 02-write_timing | pass | pass | pass |  |
| 03-modify_timing | pass | pass | pass |  |

### bully

#### bully
| Test | Expected | Actual | Status | Context |
| --- | --- | --- | --- | --- |
| bully (CGB) | pass | pass | pass |  |
| bully (DMG) | pass | pass | pass |  |

### little-things-gb

#### firstwhite
| Test | Expected | Actual | Status | Context |
| --- | --- | --- | --- | --- |
| firstwhite (CGB) | pass | pass | pass |  |
| firstwhite (DMG) | pass | pass | pass |  |

#### tellinglys
| Test | Expected | Actual | Status | Context |
| --- | --- | --- | --- | --- |
| tellinglys (CGB) | pass | pass | pass |  |
| tellinglys (DMG) | pass | pass | pass |  |

### mooneye

#### acceptance
| Test | Expected | Actual | Status | Context |
| --- | --- | --- | --- | --- |
| add_sp_e_timing | pass | pass | pass |  |
| boot_div-dmg0 | pass | pass | pass |  |
| boot_div-dmgABCmgb | pass | pass | pass |  |
| boot_div-S | pass | pass | pass |  |
| boot_div2-S | pass | pass | pass |  |
| boot_hwio-dmg0 | pass | pass | pass |  |
| boot_hwio-dmgABCmgb | pass | pass | pass |  |
| boot_hwio-S | pass | pass | pass |  |
| boot_regs-dmg0 | pass | pass | pass |  |
| boot_regs-dmgABC | pass | pass | pass |  |
| boot_regs-mgb | pass | pass | pass |  |
| boot_regs-sgb | pass | pass | pass |  |
| boot_regs-sgb2 | pass | pass | pass |  |
| call_cc_timing | pass | pass | pass |  |
| call_cc_timing2 | pass | pass | pass |  |
| call_timing | pass | pass | pass |  |
| call_timing2 | pass | pass | pass |  |
| di_timing-GS | pass | pass | pass |  |
| div_timing | pass | pass | pass |  |
| ei_sequence | pass | pass | pass |  |
| ei_timing | pass | pass | pass |  |
| halt_ime0_ei | pass | pass | pass |  |
| halt_ime0_nointr_timing | pass | pass | pass |  |
| halt_ime1_timing | pass | pass | pass |  |
| halt_ime1_timing2-GS | pass | pass | pass |  |
| if_ie_registers | pass | pass | pass |  |
| intr_timing | pass | pass | pass |  |
| jp_cc_timing | pass | pass | pass |  |
| jp_timing | pass | pass | pass |  |
| ld_hl_sp_e_timing | pass | pass | pass |  |
| oam_dma_restart | pass | pass | pass |  |
| oam_dma_start | pass | pass | pass |  |
| oam_dma_timing | pass | pass | pass |  |
| pop_timing | pass | pass | pass |  |
| push_timing | pass | pass | pass |  |
| rapid_di_ei | pass | pass | pass |  |
| ret_cc_timing | pass | pass | pass |  |
| ret_timing | pass | pass | pass |  |
| reti_intr_timing | pass | pass | pass |  |
| reti_timing | pass | pass | pass |  |
| rst_timing | pass | pass | pass |  |

#### bits
| Test | Expected | Actual | Status | Context |
| --- | --- | --- | --- | --- |
| mem_oam | pass | pass | pass |  |
| reg_f | pass | pass | pass |  |
| unused_hwio-C | pass | pass | pass |  |
| unused_hwio-GS | pass | pass | pass |  |

#### instr
| Test | Expected | Actual | Status | Context |
| --- | --- | --- | --- | --- |
| daa | pass | pass | pass |  |

#### interrupts
| Test | Expected | Actual | Status | Context |
| --- | --- | --- | --- | --- |
| ie_push | pass | pass | pass |  |

#### madness
| Test | Expected | Actual | Status | Context |
| --- | --- | --- | --- | --- |
| mgb_oam_dma_halt_sprites | fail | fail | xfail | Known failure imported from tests/README.md |

#### manual-only
| Test | Expected | Actual | Status | Context |
| --- | --- | --- | --- | --- |
| sprite_priority (CGB) | pass | pass | pass |  |
| sprite_priority (DMG) | pass | pass | pass |  |

#### mbc1
| Test | Expected | Actual | Status | Context |
| --- | --- | --- | --- | --- |
| bits_bank1 | pass | pass | pass |  |
| bits_bank2 | pass | pass | pass |  |
| bits_mode | pass | pass | pass |  |
| bits_ramg | pass | pass | pass |  |
| multicart_rom_8Mb | pass | pass | pass |  |
| ram_256kb | pass | pass | pass |  |
| ram_64kb | pass | pass | pass |  |
| rom_16Mb | pass | pass | pass |  |
| rom_1Mb | pass | pass | pass |  |
| rom_2Mb | pass | pass | pass |  |
| rom_4Mb | pass | pass | pass |  |
| rom_512kb | pass | pass | pass |  |
| rom_8Mb | pass | pass | pass |  |

#### mbc2
| Test | Expected | Actual | Status | Context |
| --- | --- | --- | --- | --- |
| bits_ramg | pass | pass | pass |  |
| bits_romb | pass | pass | pass |  |
| bits_unused | pass | pass | pass |  |
| ram | pass | pass | pass |  |
| rom_1Mb | pass | pass | pass |  |
| rom_2Mb | pass | pass | pass |  |
| rom_512kb | pass | pass | pass |  |

#### mbc5
| Test | Expected | Actual | Status | Context |
| --- | --- | --- | --- | --- |
| rom_16Mb | pass | pass | pass |  |
| rom_1Mb | pass | pass | pass |  |
| rom_2Mb | pass | pass | pass |  |
| rom_32Mb | pass | pass | pass |  |
| rom_4Mb | pass | pass | pass |  |
| rom_512kb | pass | pass | pass |  |
| rom_64Mb | pass | pass | pass |  |
| rom_8Mb | pass | pass | pass |  |

#### misc
| Test | Expected | Actual | Status | Context |
| --- | --- | --- | --- | --- |
| boot_div-A | pass | pass | pass |  |
| boot_div-cgb0 | pass | pass | pass |  |
| boot_div-cgbABCDE | pass | pass | pass |  |
| boot_hwio-C | pass | pass | pass |  |
| boot_regs-A | pass | pass | pass |  |
| boot_regs-cgb | pass | pass | pass |  |

#### oam_dma
| Test | Expected | Actual | Status | Context |
| --- | --- | --- | --- | --- |
| basic | pass | pass | pass |  |
| reg_read | pass | pass | pass |  |
| sources-GS | pass | pass | pass |  |

#### ppu
| Test | Expected | Actual | Status | Context |
| --- | --- | --- | --- | --- |
| hblank_ly_scx_timing-GS | pass | pass | pass |  |
| intr_1_2_timing-GS | pass | pass | pass |  |
| intr_2_0_timing | pass | pass | pass |  |
| intr_2_mode0_timing | pass | pass | pass |  |
| intr_2_mode0_timing_sprites | pass | pass | pass |  |
| intr_2_mode3_timing | pass | pass | pass |  |
| intr_2_oam_ok_timing | pass | pass | pass |  |
| lcdon_timing-GS | pass | pass | pass |  |
| lcdon_write_timing-GS | pass | pass | pass |  |
| stat_irq_blocking | pass | pass | pass |  |
| stat_lyc_onoff | pass | pass | pass |  |
| vblank_stat_intr-C | pass | pass | pass |  |
| vblank_stat_intr-GS | pass | pass | pass |  |

#### serial
| Test | Expected | Actual | Status | Context |
| --- | --- | --- | --- | --- |
| boot_sclk_align-dmgABCmgb | pass | pass | pass |  |

#### timer
| Test | Expected | Actual | Status | Context |
| --- | --- | --- | --- | --- |
| div_write | pass | pass | pass |  |
| rapid_toggle | pass | pass | pass |  |
| tim00 | pass | pass | pass |  |
| tim00_div_trigger | pass | pass | pass |  |
| tim01 | pass | pass | pass |  |
| tim01_div_trigger | pass | pass | pass |  |
| tim10 | pass | pass | pass |  |
| tim10_div_trigger | pass | pass | pass |  |
| tim11 | pass | pass | pass |  |
| tim11_div_trigger | pass | pass | pass |  |
| tima_reload | pass | pass | pass |  |
| tima_write_reloading | pass | pass | pass |  |
| tma_write_reloading | pass | pass | pass |  |

### samesuite

#### apu/channel_1
| Test | Expected | Actual | Status | Context |
| --- | --- | --- | --- | --- |
| channel_1_align | pass | pass | pass |  |
| channel_1_align_cpu | pass | pass | pass |  |
| channel_1_delay | pass | pass | pass |  |
| channel_1_duty | pass | pass | pass |  |
| channel_1_duty_delay | pass | pass | pass |  |
| channel_1_extra_length_clocking-cgb0B | fail | fail | xfail | Known failure imported from tests/README.md |
| channel_1_freq_change | pass | pass | pass |  |
| channel_1_freq_change_timing-A | fail | fail | xfail | Known failure imported from tests/README.md |
| channel_1_freq_change_timing-cgb0BC | fail | fail | xfail | Known failure imported from tests/README.md |
| channel_1_freq_change_timing-cgbDE | fail | fail | xfail | Known failure imported from tests/README.md |
| channel_1_nrx2_glitch | pass | pass | pass |  |
| channel_1_nrx2_speed_change | pass | pass | pass |  |
| channel_1_restart | pass | pass | pass |  |
| channel_1_restart_nrx2_glitch | pass | pass | pass |  |
| channel_1_stop_div | pass | pass | pass |  |
| channel_1_stop_restart | pass | pass | pass |  |
| channel_1_sweep | fail | fail | xfail | Known failure imported from tests/README.md |
| channel_1_sweep_restart | fail | fail | xfail | Known failure imported from tests/README.md |
| channel_1_sweep_restart_2 | fail | fail | xfail | Known failure imported from tests/README.md |
| channel_1_volume | pass | pass | pass |  |
| channel_1_volume_div | pass | pass | pass |  |

#### apu/channel_2
| Test | Expected | Actual | Status | Context |
| --- | --- | --- | --- | --- |
| channel_2_align | pass | pass | pass |  |
| channel_2_align_cpu | pass | pass | pass |  |
| channel_2_delay | pass | pass | pass |  |
| channel_2_duty | pass | pass | pass |  |
| channel_2_duty_delay | pass | pass | pass |  |
| channel_2_extra_length_clocking-cgb0B | fail | fail | xfail | Known failure imported from tests/README.md |
| channel_2_freq_change | pass | pass | pass |  |
| channel_2_nrx2_glitch | pass | pass | pass |  |
| channel_2_nrx2_speed_change | pass | pass | pass |  |
| channel_2_restart | pass | pass | pass |  |
| channel_2_restart_nrx2_glitch | pass | pass | pass |  |
| channel_2_stop_div | pass | pass | pass |  |
| channel_2_stop_restart | pass | pass | pass |  |
| channel_2_volume | pass | pass | pass |  |
| channel_2_volume_div | pass | pass | pass |  |

#### apu/channel_3
| Test | Expected | Actual | Status | Context |
| --- | --- | --- | --- | --- |
| channel_3_and_glitch | pass | pass | pass |  |
| channel_3_delay | pass | pass | pass |  |
| channel_3_extra_length_clocking-cgb0 | fail | fail | xfail | Known failure imported from tests/README.md |
| channel_3_extra_length_clocking-cgbB | fail | fail | xfail | Known failure imported from tests/README.md |
| channel_3_first_sample | pass | pass | pass |  |
| channel_3_freq_change_delay | fail | fail | xfail | Known failure imported from tests/README.md |
| channel_3_restart_delay | fail | fail | xfail | Known failure imported from tests/README.md |
| channel_3_restart_during_delay | pass | pass | pass |  |
| channel_3_restart_stop_delay | pass | pass | pass |  |
| channel_3_shift_delay | pass | pass | pass |  |
| channel_3_shift_skip_delay | pass | pass | pass |  |
| channel_3_stop_delay | pass | pass | pass |  |
| channel_3_stop_div | pass | pass | pass |  |
| channel_3_wave_ram_dac_on_rw | pass | pass | pass |  |
| channel_3_wave_ram_locked_write | pass | pass | pass |  |
| channel_3_wave_ram_sync | pass | pass | pass |  |

#### apu/channel_4
| Test | Expected | Actual | Status | Context |
| --- | --- | --- | --- | --- |
| channel_4_align | pass | pass | pass |  |
| channel_4_delay | fail | fail | xfail | Known failure imported from tests/README.md |
| channel_4_equivalent_frequencies | fail | fail | xfail | Known failure imported from tests/README.md |
| channel_4_extra_length_clocking-cgb0B | fail | fail | xfail | Known failure imported from tests/README.md |
| channel_4_freq_change | fail | fail | xfail | Known failure imported from tests/README.md |
| channel_4_frequency_alignment | fail | fail | xfail | Known failure imported from tests/README.md |
| channel_4_lfsr | pass | pass | pass |  |
| channel_4_lfsr_15_7 | pass | pass | pass |  |
| channel_4_lfsr_7_15 | pass | pass | pass |  |
| channel_4_lfsr_restart | pass | pass | pass |  |
| channel_4_lfsr_restart_fast | pass | pass | pass |  |
| channel_4_lfsr15 | pass | pass | pass |  |
| channel_4_volume_div | pass | pass | pass |  |

#### apu
| Test | Expected | Actual | Status | Context |
| --- | --- | --- | --- | --- |
| div_trigger_volume_10 | pass | pass | pass |  |
| div_write_trigger | pass | pass | pass |  |
| div_write_trigger_10 | pass | pass | pass |  |
| div_write_trigger_volume | pass | pass | pass |  |
| div_write_trigger_volume_10 | pass | pass | pass |  |

#### dma
| Test | Expected | Actual | Status | Context |
| --- | --- | --- | --- | --- |
| gbc_dma_cont | pass | pass | pass |  |
| gdma_addr_mask | pass | pass | pass |  |
| hdma_lcd_off | pass | pass | pass |  |
| hdma_mode0 | pass | pass | pass |  |

#### interrupt
| Test | Expected | Actual | Status | Context |
| --- | --- | --- | --- | --- |
| ei_delay_halt | pass | pass | pass |  |

#### ppu
| Test | Expected | Actual | Status | Context |
| --- | --- | --- | --- | --- |
| blocking_bgpi_increase | pass | pass | pass |  |

#### sgb
| Test | Expected | Actual | Status | Context |
| --- | --- | --- | --- | --- |
| command_mlt_req | fail | fail | xfail | Known failure imported from tests/README.md |
| command_mlt_req_1_incrementing | fail | fail | xfail | Known failure imported from tests/README.md |

### scribbltests

#### scribbltests
| Test | Expected | Actual | Status | Context |
| --- | --- | --- | --- | --- |
| lycscx | pass | pass | pass |  |
| lycscy | pass | pass | pass |  |
| palettely | pass | pass | pass |  |
| scxly | pass | pass | pass |  |
| statcount-auto | pass | pass | pass |  |

### strikethrough

#### strikethrough
| Test | Expected | Actual | Status | Context |
| --- | --- | --- | --- | --- |
| strikethrough (CGB) | pass | pass | pass |  |
| strikethrough (DMG) | pass | pass | pass |  |
