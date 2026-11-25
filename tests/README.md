# Automated test results

This report is generated from the structured per-test result set used by CI and evaluated against `tests/regression-baseline.json`. PASS and XFAIL match the checked-in baseline; REGRESSION, XPASS, new, missing, or model-change entries require review.

![progress](https://progress-bar.xyz/16/?scale=100&title=passing%2053,%20failing%20276&width=500)

| Status | Count |
| --- | ---: |
| PASS | 53 |
| XFAIL | 26 |
| REGRESSION | 250 |
| XPASS | 0 |
| New tests | 0 |
| Missing tests | 0 |
| Model changes | 0 |

<hr/>
GomeBoy is automatically tested against the following test suites:

* **[AGE test roms](https://github.com/c-sp/age-test-roms)**  
  <sup>by [c-sp](https://github.com/c-sp); CI uses AGE revision cd3f654d13bf from the pinned game-boy-test-roms v7.0 release.</sup>
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
* **[Mealybug Tearoom Tests](https://github.com/mattcurrie/mealybug-tearoom-tests)**  
  <sup>by Matt Currie; pinned screenshot-at-LD-B-B PPU timing subset for DMG, CGB B/C and CGB D/E.</sup>
* **[Mooneye Test Suite](https://github.com/Gekkio/mooneye-test-suite)**  
  <sup>by [Joonas Javanainen](https://github.com/Gekkio) </sup>
* **[SameSuite](https://github.com/LIJI32/SameSuite)**  
  <sup>by [Lior Halphon](https://github.com/LIJI32) </sup>

External-suite integration status, including pinned but not-yet-run candidates, is tracked in tests/external-corpus.json.

Different test suites use different pass/fail criteria. Some may write output to the serial port such as
[Blargg's test roms](https://github.com/retrio/gb-test-roms), others may write to the CPU registers, such as 
[Mooneye Test Suite](https://github.com/Gekkio/mooneye-test-suite) and [SameSuite](https://github.com/LIJI32/SameSuite).
If the test suite does not provide a way to automatically determine a pass/fail criteria, then the emulator's output
is compared against a reference image from a known good emulator.
<hr/>


# Test Results
| Test Suite | Pass Rate | Tests Passed | Tests Failed | Tests Total |
| --- | --- | --- | --- | --- |
| acid2 | 0% | 0 | 4 | 4 |
| age | 0% | 0 | 55 | 55 |
| blarrg | 0% | 0 | 43 | 43 |
| bully | 0% | 0 | 2 | 2 |
| little-things-gb | 0% | 0 | 4 | 4 |
| mealybug | 0% | 0 | 18 | 18 |
| mooneye | 46% | 53 | 61 | 114 |
| samesuite | 0% | 0 | 82 | 82 |
| scribbltests | 0% | 0 | 5 | 5 |
| strikethrough | 0% | 0 | 2 | 2 |

## Per-test results

### acid2

#### cgb-acid2
| Test | Expected | Actual | Status | Context |
| --- | --- | --- | --- | --- |
| cgb-acid-hell | pass | fail | regression | expected pass, observed failure |
| cgb-acid2 | pass | fail | regression | expected pass, observed failure |

#### dmg-acid2
| Test | Expected | Actual | Status | Context |
| --- | --- | --- | --- | --- |
| dmg-acid2 (CGB) | pass | fail | regression | expected pass, observed failure |
| dmg-acid2 (DMG) | pass | fail | regression | expected pass, observed failure |

### age

#### halt
| Test | Expected | Actual | Status | Context |
| --- | --- | --- | --- | --- |
| ei-halt-dmgC-cgbBCE@CGBBC | pass | fail | regression | expected pass, observed failure |
| ei-halt-dmgC-cgbBCE@CGBDE | pass | fail | regression | expected pass, observed failure |
| ei-halt-dmgC-cgbBCE@DMG | pass | fail | regression | expected pass, observed failure |
| halt-m0-interrupt-dmgC-cgbBCE@CGBBC | pass | fail | regression | expected pass, observed failure |
| halt-m0-interrupt-dmgC-cgbBCE@CGBDE | pass | fail | regression | expected pass, observed failure |
| halt-m0-interrupt-dmgC-cgbBCE@DMG | pass | fail | regression | expected pass, observed failure |
| halt-prefetch-dmgC-cgbBCE@CGBBC | pass | fail | regression | expected pass, observed failure |
| halt-prefetch-dmgC-cgbBCE@CGBDE | pass | fail | regression | expected pass, observed failure |
| halt-prefetch-dmgC-cgbBCE@DMG | pass | fail | regression | expected pass, observed failure |

#### lcd-align-ly
| Test | Expected | Actual | Status | Context |
| --- | --- | --- | --- | --- |
| lcd-align-ly-cgbBC@CGBBC | fail | fail | xfail | Known AGE v7.0 failure at pinned cd3f654d13bf baseline |
| lcd-align-ly-cgbE@CGBDE | fail | fail | xfail | Known AGE v7.0 failure at pinned cd3f654d13bf baseline |

#### ly
| Test | Expected | Actual | Status | Context |
| --- | --- | --- | --- | --- |
| ly-cgbE@CGBDE | pass | fail | regression | expected pass, observed failure |
| ly-dmgC-cgbBC@CGBBC | pass | fail | regression | expected pass, observed failure |
| ly-dmgC-cgbBC@DMG | pass | fail | regression | expected pass, observed failure |
| ly-ncmBC@CGBBC | pass | fail | regression | expected pass, observed failure |
| ly-ncmE@CGBDE | pass | fail | regression | expected pass, observed failure |

#### oam
| Test | Expected | Actual | Status | Context |
| --- | --- | --- | --- | --- |
| oam-read-cgbE@CGBDE | pass | fail | regression | expected pass, observed failure |
| oam-read-dmgC-cgbBC@CGBBC | pass | fail | regression | expected pass, observed failure |
| oam-read-dmgC-cgbBC@DMG | pass | fail | regression | expected pass, observed failure |
| oam-read-ncmBC@CGBBC | pass | fail | regression | expected pass, observed failure |
| oam-read-ncmE@CGBDE | pass | fail | regression | expected pass, observed failure |
| oam-write-cgbBCE@CGBBC | pass | fail | regression | expected pass, observed failure |
| oam-write-cgbBCE@CGBDE | pass | fail | regression | expected pass, observed failure |
| oam-write-dmgC@DMG | fail | fail | xfail | Known AGE v7.0 failure at pinned cd3f654d13bf baseline |
| oam-write-ncmBCE@CGBBC | pass | fail | regression | expected pass, observed failure |
| oam-write-ncmBCE@CGBDE | pass | fail | regression | expected pass, observed failure |

#### stat-interrupt
| Test | Expected | Actual | Status | Context |
| --- | --- | --- | --- | --- |
| stat-int-dmgC-cgbBCE@CGBBC | fail | fail | xfail | Known AGE v7.0 failure at pinned cd3f654d13bf baseline |
| stat-int-dmgC-cgbBCE@CGBDE | fail | fail | xfail | Known AGE v7.0 failure at pinned cd3f654d13bf baseline |
| stat-int-dmgC-cgbBCE@DMG | fail | fail | xfail | Known AGE v7.0 failure at pinned cd3f654d13bf baseline |
| stat-int-ncmBCE@CGBBC | fail | fail | xfail | Known AGE v7.0 failure at pinned cd3f654d13bf baseline |
| stat-int-ncmBCE@CGBDE | fail | fail | xfail | Known AGE v7.0 failure at pinned cd3f654d13bf baseline |

#### stat-mode-sprites
| Test | Expected | Actual | Status | Context |
| --- | --- | --- | --- | --- |
| stat-mode-sprites-dmgC-cgbBCE@CGBBC | fail | fail | xfail | Known AGE v7.0 failure at pinned cd3f654d13bf baseline |
| stat-mode-sprites-dmgC-cgbBCE@CGBDE | fail | fail | xfail | Known AGE v7.0 failure at pinned cd3f654d13bf baseline |
| stat-mode-sprites-dmgC-cgbBCE@DMG | fail | fail | xfail | Known AGE v7.0 failure at pinned cd3f654d13bf baseline |
| stat-mode-sprites-ds-cgbBCE@CGBBC | fail | fail | xfail | Known AGE v7.0 failure at pinned cd3f654d13bf baseline |
| stat-mode-sprites-ds-cgbBCE@CGBDE | fail | fail | xfail | Known AGE v7.0 failure at pinned cd3f654d13bf baseline |

#### stat-mode-window
| Test | Expected | Actual | Status | Context |
| --- | --- | --- | --- | --- |
| stat-mode-window-cgbBCE@CGBBC | fail | fail | xfail | Known AGE v7.0 failure at pinned cd3f654d13bf baseline |
| stat-mode-window-cgbBCE@CGBDE | fail | fail | xfail | Known AGE v7.0 failure at pinned cd3f654d13bf baseline |
| stat-mode-window-dmgC@DMG | fail | fail | xfail | Known AGE v7.0 failure at pinned cd3f654d13bf baseline |
| stat-mode-window-ds-cgbBCE@CGBBC | fail | fail | xfail | Known AGE v7.0 failure at pinned cd3f654d13bf baseline |
| stat-mode-window-ds-cgbBCE@CGBDE | fail | fail | xfail | Known AGE v7.0 failure at pinned cd3f654d13bf baseline |
| stat-mode-window-ncmBCE@CGBBC | fail | fail | xfail | Known AGE v7.0 failure at pinned cd3f654d13bf baseline |
| stat-mode-window-ncmBCE@CGBDE | fail | fail | xfail | Known AGE v7.0 failure at pinned cd3f654d13bf baseline |

#### stat-mode
| Test | Expected | Actual | Status | Context |
| --- | --- | --- | --- | --- |
| stat-mode-cgbE@CGBDE | pass | fail | regression | expected pass, observed failure |
| stat-mode-dmgC-cgbBC@CGBBC | pass | fail | regression | expected pass, observed failure |
| stat-mode-dmgC-cgbBC@DMG | pass | fail | regression | expected pass, observed failure |
| stat-mode-ds-cgbBCE@CGBBC | pass | fail | regression | expected pass, observed failure |
| stat-mode-ds-cgbBCE@CGBDE | pass | fail | regression | expected pass, observed failure |
| stat-mode-ncmBC@CGBBC | pass | fail | regression | expected pass, observed failure |
| stat-mode-ncmE@CGBDE | pass | fail | regression | expected pass, observed failure |

#### vram
| Test | Expected | Actual | Status | Context |
| --- | --- | --- | --- | --- |
| vram-read-cgbBCE@CGBBC | pass | fail | regression | expected pass, observed failure |
| vram-read-cgbBCE@CGBDE | pass | fail | regression | expected pass, observed failure |
| vram-read-dmgC@DMG | pass | fail | regression | expected pass, observed failure |
| vram-read-ncmBCE@CGBBC | pass | fail | regression | expected pass, observed failure |
| vram-read-ncmBCE@CGBDE | pass | fail | regression | expected pass, observed failure |

### blarrg

#### cgb_sound
| Test | Expected | Actual | Status | Context |
| --- | --- | --- | --- | --- |
| 01-registers | pass | fail | regression | expected pass, observed failure |
| 02-len ctr | pass | fail | regression | expected pass, observed failure |
| 03-trigger | pass | fail | regression | expected pass, observed failure |
| 04-sweep | pass | fail | regression | expected pass, observed failure |
| 05-sweep details | pass | fail | regression | expected pass, observed failure |
| 06-overflow on trigger | pass | fail | regression | expected pass, observed failure |
| 07-len sweep period sync | pass | fail | regression | expected pass, observed failure |
| 08-len ctr during power | pass | fail | regression | expected pass, observed failure |
| 09-wave read while on | pass | fail | regression | expected pass, observed failure |
| 10-wave trigger while on | pass | fail | regression | expected pass, observed failure |
| 11-regs after power | pass | fail | regression | expected pass, observed failure |
| 12-wave | pass | fail | regression | expected pass, observed failure |

#### cpu_instrs
| Test | Expected | Actual | Status | Context |
| --- | --- | --- | --- | --- |
| 01-special | pass | fail | regression | expected pass, observed failure |
| 02-interrupts | pass | fail | regression | expected pass, observed failure |
| 03-op sp,hl | pass | fail | regression | expected pass, observed failure |
| 04-op r,imm | pass | fail | regression | expected pass, observed failure |
| 05-op rp | pass | fail | regression | expected pass, observed failure |
| 06-ld r,r | pass | fail | regression | expected pass, observed failure |
| 07-jr,jp,call,ret,rst | pass | fail | regression | expected pass, observed failure |
| 08-misc instrs | pass | fail | regression | expected pass, observed failure |
| 09-op r,r | pass | fail | regression | expected pass, observed failure |
| 10-bit ops | pass | fail | regression | expected pass, observed failure |
| 11-op a,(hl) | pass | fail | regression | expected pass, observed failure |

#### dmg_sound
| Test | Expected | Actual | Status | Context |
| --- | --- | --- | --- | --- |
| 01-registers | pass | fail | regression | expected pass, observed failure |
| 02-len ctr | pass | fail | regression | expected pass, observed failure |
| 03-trigger | pass | fail | regression | expected pass, observed failure |
| 04-sweep | pass | fail | regression | expected pass, observed failure |
| 05-sweep details | pass | fail | regression | expected pass, observed failure |
| 06-overflow on trigger | pass | fail | regression | expected pass, observed failure |
| 07-len sweep period sync | pass | fail | regression | expected pass, observed failure |
| 08-len ctr during power | pass | fail | regression | expected pass, observed failure |
| 09-wave read while on | pass | fail | regression | expected pass, observed failure |
| 10-wave trigger while on | pass | fail | regression | expected pass, observed failure |
| 11-regs after power | pass | fail | regression | expected pass, observed failure |
| 12-wave write while on | pass | fail | regression | expected pass, observed failure |

#### halt_bug
| Test | Expected | Actual | Status | Context |
| --- | --- | --- | --- | --- |
| halt_bug (CGB) | pass | fail | regression | expected pass, observed failure |
| halt_bug (DMG) | pass | fail | regression | expected pass, observed failure |

#### instr_timing
| Test | Expected | Actual | Status | Context |
| --- | --- | --- | --- | --- |
| instr_timing | pass | fail | regression | expected pass, observed failure |

#### interrupt_time
| Test | Expected | Actual | Status | Context |
| --- | --- | --- | --- | --- |
| interrupt_time (CGB) | pass | fail | regression | expected pass, observed failure |
| interrupt_time (DMG) | pass | fail | regression | expected pass, observed failure |

#### mem_timing
| Test | Expected | Actual | Status | Context |
| --- | --- | --- | --- | --- |
| 01-read_timing | pass | fail | regression | expected pass, observed failure |
| 02-write_timing | pass | fail | regression | expected pass, observed failure |
| 03-modify_timing | pass | fail | regression | expected pass, observed failure |

### bully

#### bully
| Test | Expected | Actual | Status | Context |
| --- | --- | --- | --- | --- |
| bully (CGB) | pass | fail | regression | expected pass, observed failure |
| bully (DMG) | pass | fail | regression | expected pass, observed failure |

### little-things-gb

#### firstwhite
| Test | Expected | Actual | Status | Context |
| --- | --- | --- | --- | --- |
| firstwhite (CGB) | pass | fail | regression | expected pass, observed failure |
| firstwhite (DMG) | pass | fail | regression | expected pass, observed failure |

#### tellinglys
| Test | Expected | Actual | Status | Context |
| --- | --- | --- | --- | --- |
| tellinglys (CGB) | pass | fail | regression | expected pass, observed failure |
| tellinglys (DMG) | pass | fail | regression | expected pass, observed failure |

### mealybug

#### ppu
| Test | Expected | Actual | Status | Context |
| --- | --- | --- | --- | --- |
| m2_win_en_toggle@CGBBC | pass | fail | regression | expected pass, observed failure |
| m2_win_en_toggle@CGBDE | pass | fail | regression | expected pass, observed failure |
| m2_win_en_toggle@DMG | pass | fail | regression | expected pass, observed failure |
| m3_bgp_change@CGBBC | fail | fail | xfail | Measured Mealybug PPU screenshot mismatch: 496 RGB pixels against pinned CGBBC reference at LD B,B (revision 70e88fb90b59d) |
| m3_bgp_change@CGBDE | fail | fail | xfail | Measured Mealybug PPU screenshot mismatch: 496 RGB pixels against pinned CGBDE reference at LD B,B (revision 70e88fb90b59d) |
| m3_bgp_change@DMG | pass | fail | regression | expected pass, observed failure |
| m3_lcdc_win_en_change_multiple@CGBBC | pass | fail | regression | expected pass, observed failure |
| m3_lcdc_win_en_change_multiple@CGBDE | pass | fail | regression | expected pass, observed failure |
| m3_lcdc_win_en_change_multiple@DMG | pass | fail | regression | expected pass, observed failure |
| m3_scx_low_3_bits@CGBBC | pass | fail | regression | expected pass, observed failure |
| m3_scx_low_3_bits@CGBDE | pass | fail | regression | expected pass, observed failure |
| m3_scx_low_3_bits@DMG | pass | fail | regression | expected pass, observed failure |
| m3_window_timing@CGBBC | fail | fail | xfail | Measured Mealybug PPU screenshot mismatch: 138 RGB pixels against pinned CGBBC reference at LD B,B (revision 70e88fb90b59d) |
| m3_window_timing@CGBDE | pass | fail | regression | expected pass, observed failure |
| m3_window_timing@DMG | pass | fail | regression | expected pass, observed failure |
| m3_wx_6_change@CGBBC | fail | fail | xfail | Measured Mealybug PPU screenshot mismatch: 19819 RGB pixels against pinned CGBBC reference at LD B,B (revision 70e88fb90b59d) |
| m3_wx_6_change@CGBDE | fail | fail | xfail | Measured Mealybug PPU screenshot mismatch: 19819 RGB pixels against pinned CGBDE reference at LD B,B (revision 70e88fb90b59d) |
| m3_wx_6_change@DMG | fail | fail | xfail | Measured Mealybug PPU screenshot mismatch: 13799 RGB pixels against pinned DMG reference at LD B,B (revision 70e88fb90b59d) |

### mooneye

#### acceptance
| Test | Expected | Actual | Status | Context |
| --- | --- | --- | --- | --- |
| add_sp_e_timing | pass | pass | pass |  |
| boot_div-S | pass | pass | pass |  |
| boot_div-dmg0 | pass | pass | pass |  |
| boot_div-dmgABCmgb | pass | pass | pass |  |
| boot_div2-S | pass | pass | pass |  |
| boot_hwio-S | pass | pass | pass |  |
| boot_hwio-dmg0 | pass | pass | pass |  |
| boot_hwio-dmgABCmgb | pass | pass | pass |  |
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
| mem_oam | pass | fail | regression | expected pass, observed failure |
| reg_f | pass | fail | regression | expected pass, observed failure |
| unused_hwio-C | pass | fail | regression | expected pass, observed failure |
| unused_hwio-GS | pass | fail | regression | expected pass, observed failure |

#### instr
| Test | Expected | Actual | Status | Context |
| --- | --- | --- | --- | --- |
| daa | pass | fail | regression | expected pass, observed failure |

#### interrupts
| Test | Expected | Actual | Status | Context |
| --- | --- | --- | --- | --- |
| ie_push | pass | fail | regression | expected pass, observed failure |

#### madness
| Test | Expected | Actual | Status | Context |
| --- | --- | --- | --- | --- |
| mgb_oam_dma_halt_sprites | pass | fail | regression | expected pass, observed failure |

#### manual-only
| Test | Expected | Actual | Status | Context |
| --- | --- | --- | --- | --- |
| sprite_priority (CGB) | pass | fail | regression | expected pass, observed failure |
| sprite_priority (DMG) | pass | fail | regression | expected pass, observed failure |

#### mbc1
| Test | Expected | Actual | Status | Context |
| --- | --- | --- | --- | --- |
| bits_bank1 | pass | fail | regression | expected pass, observed failure |
| bits_bank2 | pass | fail | regression | expected pass, observed failure |
| bits_mode | pass | fail | regression | expected pass, observed failure |
| bits_ramg | pass | fail | regression | expected pass, observed failure |
| multicart_rom_8Mb | pass | fail | regression | expected pass, observed failure |
| ram_256kb | pass | fail | regression | expected pass, observed failure |
| ram_64kb | pass | fail | regression | expected pass, observed failure |
| rom_16Mb | pass | fail | regression | expected pass, observed failure |
| rom_1Mb | pass | fail | regression | expected pass, observed failure |
| rom_2Mb | pass | fail | regression | expected pass, observed failure |
| rom_4Mb | pass | fail | regression | expected pass, observed failure |
| rom_512kb | pass | fail | regression | expected pass, observed failure |
| rom_8Mb | pass | fail | regression | expected pass, observed failure |

#### mbc2
| Test | Expected | Actual | Status | Context |
| --- | --- | --- | --- | --- |
| bits_ramg | pass | fail | regression | expected pass, observed failure |
| bits_romb | pass | fail | regression | expected pass, observed failure |
| bits_unused | pass | fail | regression | expected pass, observed failure |
| ram | pass | fail | regression | expected pass, observed failure |
| rom_1Mb | pass | fail | regression | expected pass, observed failure |
| rom_2Mb | pass | fail | regression | expected pass, observed failure |
| rom_512kb | pass | fail | regression | expected pass, observed failure |

#### mbc5
| Test | Expected | Actual | Status | Context |
| --- | --- | --- | --- | --- |
| rom_16Mb | pass | fail | regression | expected pass, observed failure |
| rom_1Mb | pass | fail | regression | expected pass, observed failure |
| rom_2Mb | pass | fail | regression | expected pass, observed failure |
| rom_32Mb | pass | fail | regression | expected pass, observed failure |
| rom_4Mb | pass | fail | regression | expected pass, observed failure |
| rom_512kb | pass | fail | regression | expected pass, observed failure |
| rom_64Mb | pass | fail | regression | expected pass, observed failure |
| rom_8Mb | pass | fail | regression | expected pass, observed failure |

#### misc
| Test | Expected | Actual | Status | Context |
| --- | --- | --- | --- | --- |
| boot_div-A | pass | fail | regression | expected pass, observed failure |
| boot_div-cgb0 | pass | fail | regression | expected pass, observed failure |
| boot_div-cgbABCDE | pass | fail | regression | expected pass, observed failure |
| boot_hwio-C | pass | fail | regression | expected pass, observed failure |
| boot_regs-A | pass | fail | regression | expected pass, observed failure |
| boot_regs-cgb | pass | fail | regression | expected pass, observed failure |

#### oam_dma
| Test | Expected | Actual | Status | Context |
| --- | --- | --- | --- | --- |
| basic | pass | fail | regression | expected pass, observed failure |
| reg_read | pass | fail | regression | expected pass, observed failure |
| sources-GS | pass | fail | regression | expected pass, observed failure |

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
| vblank_stat_intr-C | pass | fail | regression | expected pass, observed failure |
| vblank_stat_intr-GS | pass | pass | pass |  |

#### serial
| Test | Expected | Actual | Status | Context |
| --- | --- | --- | --- | --- |
| boot_sclk_align-dmgABCmgb | pass | fail | regression | expected pass, observed failure |

#### timer
| Test | Expected | Actual | Status | Context |
| --- | --- | --- | --- | --- |
| div_write | pass | fail | regression | expected pass, observed failure |
| rapid_toggle | pass | fail | regression | expected pass, observed failure |
| tim00 | pass | fail | regression | expected pass, observed failure |
| tim00_div_trigger | pass | fail | regression | expected pass, observed failure |
| tim01 | pass | fail | regression | expected pass, observed failure |
| tim01_div_trigger | pass | fail | regression | expected pass, observed failure |
| tim10 | pass | fail | regression | expected pass, observed failure |
| tim10_div_trigger | pass | fail | regression | expected pass, observed failure |
| tim11 | pass | fail | regression | expected pass, observed failure |
| tim11_div_trigger | pass | fail | regression | expected pass, observed failure |
| tima_reload | pass | fail | regression | expected pass, observed failure |
| tima_write_reloading | pass | fail | regression | expected pass, observed failure |
| tma_write_reloading | pass | fail | regression | expected pass, observed failure |

### samesuite

#### apu/channel_1
| Test | Expected | Actual | Status | Context |
| --- | --- | --- | --- | --- |
| channel_1_align | pass | fail | regression | expected pass, observed failure |
| channel_1_align_cpu | pass | fail | regression | expected pass, observed failure |
| channel_1_delay | pass | fail | regression | expected pass, observed failure |
| channel_1_duty | pass | fail | regression | expected pass, observed failure |
| channel_1_duty_delay | pass | fail | regression | expected pass, observed failure |
| channel_1_extra_length_clocking-cgb0B@CGB0 | pass | fail | regression | expected pass, observed failure |
| channel_1_extra_length_clocking-cgb0B@CGBBC | pass | fail | regression | expected pass, observed failure |
| channel_1_freq_change | pass | fail | regression | expected pass, observed failure |
| channel_1_freq_change_timing-A | pass | fail | regression | expected pass, observed failure |
| channel_1_freq_change_timing-cgb0BC@CGB0 | pass | fail | regression | expected pass, observed failure |
| channel_1_freq_change_timing-cgb0BC@CGBBC | pass | fail | regression | expected pass, observed failure |
| channel_1_freq_change_timing-cgbDE | pass | fail | regression | expected pass, observed failure |
| channel_1_nrx2_glitch | pass | fail | regression | expected pass, observed failure |
| channel_1_nrx2_speed_change | pass | fail | regression | expected pass, observed failure |
| channel_1_restart | pass | fail | regression | expected pass, observed failure |
| channel_1_restart_nrx2_glitch | pass | fail | regression | expected pass, observed failure |
| channel_1_stop_div | pass | fail | regression | expected pass, observed failure |
| channel_1_stop_restart | pass | fail | regression | expected pass, observed failure |
| channel_1_sweep | pass | fail | regression | expected pass, observed failure |
| channel_1_sweep_restart | pass | fail | regression | expected pass, observed failure |
| channel_1_sweep_restart_2 | pass | fail | regression | expected pass, observed failure |
| channel_1_volume | pass | fail | regression | expected pass, observed failure |
| channel_1_volume_div | pass | fail | regression | expected pass, observed failure |

#### apu/channel_2
| Test | Expected | Actual | Status | Context |
| --- | --- | --- | --- | --- |
| channel_2_align | pass | fail | regression | expected pass, observed failure |
| channel_2_align_cpu | pass | fail | regression | expected pass, observed failure |
| channel_2_delay | pass | fail | regression | expected pass, observed failure |
| channel_2_duty | pass | fail | regression | expected pass, observed failure |
| channel_2_duty_delay | pass | fail | regression | expected pass, observed failure |
| channel_2_extra_length_clocking-cgb0B@CGB0 | pass | fail | regression | expected pass, observed failure |
| channel_2_extra_length_clocking-cgb0B@CGBBC | pass | fail | regression | expected pass, observed failure |
| channel_2_freq_change | pass | fail | regression | expected pass, observed failure |
| channel_2_nrx2_glitch | pass | fail | regression | expected pass, observed failure |
| channel_2_nrx2_speed_change | pass | fail | regression | expected pass, observed failure |
| channel_2_restart | pass | fail | regression | expected pass, observed failure |
| channel_2_restart_nrx2_glitch | pass | fail | regression | expected pass, observed failure |
| channel_2_stop_div | pass | fail | regression | expected pass, observed failure |
| channel_2_stop_restart | pass | fail | regression | expected pass, observed failure |
| channel_2_volume | pass | fail | regression | expected pass, observed failure |
| channel_2_volume_div | pass | fail | regression | expected pass, observed failure |

#### apu/channel_3
| Test | Expected | Actual | Status | Context |
| --- | --- | --- | --- | --- |
| channel_3_and_glitch | pass | fail | regression | expected pass, observed failure |
| channel_3_delay | pass | fail | regression | expected pass, observed failure |
| channel_3_extra_length_clocking-cgb0 | pass | fail | regression | expected pass, observed failure |
| channel_3_extra_length_clocking-cgbB | pass | fail | regression | expected pass, observed failure |
| channel_3_first_sample | pass | fail | regression | expected pass, observed failure |
| channel_3_freq_change_delay | pass | fail | regression | expected pass, observed failure |
| channel_3_restart_delay | pass | fail | regression | expected pass, observed failure |
| channel_3_restart_during_delay | pass | fail | regression | expected pass, observed failure |
| channel_3_restart_stop_delay | pass | fail | regression | expected pass, observed failure |
| channel_3_shift_delay | pass | fail | regression | expected pass, observed failure |
| channel_3_shift_skip_delay | pass | fail | regression | expected pass, observed failure |
| channel_3_stop_delay | pass | fail | regression | expected pass, observed failure |
| channel_3_stop_div | pass | fail | regression | expected pass, observed failure |
| channel_3_wave_ram_dac_on_rw | pass | fail | regression | expected pass, observed failure |
| channel_3_wave_ram_locked_write | pass | fail | regression | expected pass, observed failure |
| channel_3_wave_ram_sync | pass | fail | regression | expected pass, observed failure |

#### apu/channel_4
| Test | Expected | Actual | Status | Context |
| --- | --- | --- | --- | --- |
| channel_4_align | pass | fail | regression | expected pass, observed failure |
| channel_4_delay | pass | fail | regression | expected pass, observed failure |
| channel_4_equivalent_frequencies | pass | fail | regression | expected pass, observed failure |
| channel_4_extra_length_clocking-cgb0B@CGB0 | pass | fail | regression | expected pass, observed failure |
| channel_4_extra_length_clocking-cgb0B@CGBBC | pass | fail | regression | expected pass, observed failure |
| channel_4_freq_change | pass | fail | regression | expected pass, observed failure |
| channel_4_frequency_alignment | pass | fail | regression | expected pass, observed failure |
| channel_4_lfsr | pass | fail | regression | expected pass, observed failure |
| channel_4_lfsr15 | pass | fail | regression | expected pass, observed failure |
| channel_4_lfsr_15_7 | pass | fail | regression | expected pass, observed failure |
| channel_4_lfsr_7_15 | pass | fail | regression | expected pass, observed failure |
| channel_4_lfsr_restart | pass | fail | regression | expected pass, observed failure |
| channel_4_lfsr_restart_fast | pass | fail | regression | expected pass, observed failure |
| channel_4_volume_div | pass | fail | regression | expected pass, observed failure |

#### apu
| Test | Expected | Actual | Status | Context |
| --- | --- | --- | --- | --- |
| div_trigger_volume_10 | pass | fail | regression | expected pass, observed failure |
| div_write_trigger | pass | fail | regression | expected pass, observed failure |
| div_write_trigger_10 | pass | fail | regression | expected pass, observed failure |
| div_write_trigger_volume | pass | fail | regression | expected pass, observed failure |
| div_write_trigger_volume_10 | pass | fail | regression | expected pass, observed failure |

#### dma
| Test | Expected | Actual | Status | Context |
| --- | --- | --- | --- | --- |
| gbc_dma_cont | pass | fail | regression | expected pass, observed failure |
| gdma_addr_mask | pass | fail | regression | expected pass, observed failure |
| hdma_lcd_off | pass | fail | regression | expected pass, observed failure |
| hdma_mode0 | pass | fail | regression | expected pass, observed failure |

#### interrupt
| Test | Expected | Actual | Status | Context |
| --- | --- | --- | --- | --- |
| ei_delay_halt | pass | fail | regression | expected pass, observed failure |

#### ppu
| Test | Expected | Actual | Status | Context |
| --- | --- | --- | --- | --- |
| blocking_bgpi_increase | pass | fail | regression | expected pass, observed failure |

#### sgb
| Test | Expected | Actual | Status | Context |
| --- | --- | --- | --- | --- |
| command_mlt_req | pass | fail | regression | expected pass, observed failure |
| command_mlt_req_1_incrementing | pass | fail | regression | expected pass, observed failure |

### scribbltests

#### scribbltests
| Test | Expected | Actual | Status | Context |
| --- | --- | --- | --- | --- |
| lycscx | pass | fail | regression | expected pass, observed failure |
| lycscy | pass | fail | regression | expected pass, observed failure |
| palettely | pass | fail | regression | expected pass, observed failure |
| scxly | pass | fail | regression | expected pass, observed failure |
| statcount-auto | pass | fail | regression | expected pass, observed failure |

### strikethrough

#### strikethrough
| Test | Expected | Actual | Status | Context |
| --- | --- | --- | --- | --- |
| strikethrough (CGB) | pass | fail | regression | expected pass, observed failure |
| strikethrough (DMG) | pass | fail | regression | expected pass, observed failure |
