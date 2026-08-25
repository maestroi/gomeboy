package tests

import (
	"context"
	"fmt"
	"github.com/thelolagemann/gomeboy/internal/gameboy"
	"github.com/thelolagemann/gomeboy/internal/types"
	"github.com/thelolagemann/gomeboy/pkg/utils"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"
)

func init() {
	// check to see if roms exists
	if _, err := os.Stat("roms"); err != nil {
		// extract roms from roms.zip
		if err := utils.Unzip("roms.zip", "roms"); err != nil {
			panic(err)
		}
	}
}

const readmeBlurb = `<hr/>
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

`

var (
	_, c, _, _   = runtime.Caller(0)
	basePath     = filepath.Dir(c)
	parseTableRE = regexp.MustCompile(`\| ([a-zA-Z0-9-]+) \| ([0-9]+%) \| ([0-9]+) \| ([0-9]+) \| ([0-9]+) \|`)
	findTableRE  = regexp.MustCompile(`(?s)\| Test Suite.*\|(.*?)`)
	progressRE   = regexp.MustCompile(`!\[progress].*?\)`)
)

// Test_All and Test_Regressions live in regressions_test.go behind the
// "test" build tag: they are slow, network-dependent, and rewrite
// tests/README.md and the main README.md, so they are excluded from the
// default `go test ./...` context.

var testers = []func(*TestTable){testAcid2, testBully, testBlarrg, testLittleThings, testMooneye, testSamesuite, testScribbl, testStrikethrough}

func testAllTable() *TestTable {
	testTable := &TestTable{
		testSuites: make([]*TestSuite, 0),
	}
	for _, t := range testers {
		t(testTable)
	}

	return testTable
}

// TestTable is a collection of many TestSuite(s).
type TestTable struct {
	// Top level tests
	testSuites []*TestSuite
}

func createProgressBar(suite *TestSuite) string {
	total := 0
	passed := 0
	for _, collection := range suite.AllCollections() {
		for _, test := range collection.tests {
			total++
			if test.Passed() {
				passed++
			}
		}
	}

	passRate := float64(passed) / float64(total)

	progressBar := fmt.Sprintf(
		"![progress](https://progress-bar.xyz/%s/?scale=100&title=passing%%20%s,%%20failing%%20%s&width=500)",
		fmt.Sprintf("%d", int(passRate*100)),
		fmt.Sprintf("%d", passed),
		fmt.Sprintf("%d", total-passed))

	return progressBar
}

func (t *TestTable) createTestResultsTable() string {
	str := "| Test Suite | Pass Rate | Tests Passed | Tests Failed | Tests Total |\n| --- | --- | --- | --- | --- |"
	for _, suite := range t.testSuites {
		str += suite.CreateTableEntry()
	}

	return str
}

func (t *TestTable) createProgressBar() string {
	passed := 0
	total := 0
	for _, suite := range t.testSuites {
		for _, collection := range suite.AllCollections() {
			for _, test := range collection.tests {
				total++
				if test.Passed() {
					passed++
				}
			}
		}
	}
	passRate := float64(passed) / float64(total)
	progressBar := fmt.Sprintf(
		"![progress](https://progress-bar.xyz/%s/?scale=100&title=passing%%20%s,%%20failing%%20%s&width=500)",
		fmt.Sprintf("%d", int(passRate*100)),
		fmt.Sprintf("%d", passed),
		fmt.Sprintf("%d", total-passed))

	return progressBar
}

func (t *TestTable) CreateReadme() string {
	tableOfContents := "# Test Results\n"
	// create the table of contents with links
	tableOfContents += t.createTestResultsTable()
	tableOfContents += "\n\nExplore the individual tests for each suite using the table of contents below.\n\n## Table of Contents\n"
	for _, suite := range t.testSuites {
		tableOfContents += "* [" + suite.name + "](#" + suite.name + ")\n"
		for _, collection := range suite.collections {
			tableOfContents += "  * [" + collection.name + "](#" + collection.name + ")\n"
			// check for subcollections
			for _, sub := range collection.subCollections {
				tableOfContents += "    * [" + sub.name + "](#" + sub.name + ")\n"
			}
		}
	}

	// create the test results
	table := ""
	for _, suite := range t.testSuites {
		table += "# " + suite.name + "\n"
		table += createProgressBar(suite) + "\n"
		for _, collection := range suite.AllCollections() {
			if len(suite.AllCollections()) > 1 {
				table += "## " + collection.name + "\n"
			} else {
				table += "\n"
			}
			table += CreateMarkdownTableFromTests(collection.tests)
		}
	}

	// create document timestamp and commit hash
	commitHash := "unknown"
	if commitHashBytes, err := exec.Command("git", "rev-parse", "HEAD").Output(); err == nil {
		// get the first 8 characters of the commit hash
		commitHash = string(commitHashBytes[:8])
	}

	// create formatted timestamp
	timeStr := fmt.Sprintf("#### This document was automatically generated from commit %s\n", commitHash)
	return `# Automated test results
` + t.createProgressBar() + "\n\n" + timeStr + readmeBlurb + "\n" + tableOfContents + "\n" + table
}

// TestSuite is a collection of tests (often by a single author, or for a single
// feature) that can be run together.
type TestSuite struct {
	name        string
	collections []*TestCollection
}

func (t *TestSuite) AllCollections() []*TestCollection {
	tests := []*TestCollection{}
	for _, collection := range t.collections {
		tests = append(tests, collection)

		for _, subCollection := range collection.subCollections {
			// TODO recursively get all sub-collections
			tests = append(tests, subCollection)

		}
	}

	return tests
}

func (t *TestSuite) NewTestCollection(name string) *TestCollection {
	collection := &TestCollection{name: name, tests: make([]ROMTest, 0)}
	t.collections = append(t.collections, collection)
	return collection
}

func (t *TestSuite) CreateTableEntry() string {
	total := 0
	passed := 0
	for _, collection := range t.AllCollections() {
		for _, test := range collection.tests {
			total++
			if test.Passed() {
				passed++
			}
		}
	}

	passRate := float64(passed) / float64(total)

	return fmt.Sprintf("\n| %s | %s | %d | %d | %d |", t.name, fmt.Sprintf("%d%%", int(passRate*100)), passed, total-passed, total)
}

func (t *TestTable) NewTestSuite(name string) *TestSuite {
	suite := &TestSuite{name: name, collections: make([]*TestCollection, 0)}
	t.testSuites = append(t.testSuites, suite)
	return suite
}

type TestCollection struct {
	tests          []ROMTest
	name           string
	subCollections []*TestCollection
}

func (tC *TestCollection) AddTests(tests ...ROMTest) {
	for _, test := range tests {
		tC.tests = append(tC.tests, test)
	}
}

// Run runs all the tests in the collection, including any tests in sub-collections.
func (tC *TestCollection) Run(t *testing.T) {
	for _, test := range tC.tests {
		test.Run(t)
	}
	for _, subCollection := range tC.subCollections {
		t.Run(subCollection.name, func(t *testing.T) {
			subCollection.Run(t)
		})
	}
}

func (tC *TestCollection) NewTestCollection(name string) *TestCollection {
	collection := &TestCollection{name: name, tests: make([]ROMTest, 0)}
	tC.subCollections = append(tC.subCollections, collection)
	return collection
}

type ROMTest interface {
	Run(t *testing.T)
	Passed() bool
	Name() string
}

func CreateMarkdownTableFromTests(tests []ROMTest) string {
	table := "| Test | Passing |\n| ---- | ------- |\n"
	for _, test := range tests {
		// pass is green check, fail is red x
		pass := "✅"
		if !test.Passed() {
			pass = "❌"
		}
		table += "| " + test.Name() + " | " + pass + " |\n"
	}
	return table
}

func testROMs(t *testing.T, roms ...ROMTest) {
	for _, rom := range roms {
		rom.Run(t)
	}
}

// basicTest
type basicTest struct {
	romPath string
	name    string
	passed  bool
	model   types.Model
}

func newBasicTest(path string, model types.Model) *basicTest {
	return &basicTest{
		romPath: path,
		name:    strings.Split(filepath.Base(path), ".")[0],
		model:   model,
	}
}

func (b *basicTest) Passed() bool {
	return b.passed
}

func (b *basicTest) Name() string {
	return b.name
}

// breakpointStrategy defines the type of breakpoint to hit.
type breakpointStrategy int

const (
	// DebugBreakpoint is a strategy that runs the Game Boy until the
	// CPU.DebugBreakpoint is reached.
	DebugBreakpoint breakpointStrategy = iota
	// CycleBreakpoint is a strategy that runs the Game Boy until the
	// scheduler.Cycle() is greater than the timeout.
	CycleBreakpoint
)

// runGameboy runs a gameboy until it hits a breakpoint as defined
// by the breakpointStrategy. It returns the gameboy instance after
// the breakpoint is hit.
func runGameboy(romPath string, timeout int, strat breakpointStrategy, opts ...gameboy.Opt) (*gameboy.GameBoy, error) {
	// create the gameboy instance
	g := gameboy.NewGameBoy(opts...)
	if err := g.LoadROM(romPath); err != nil {
		return nil, err
	}

	// create timeout
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
	defer cancel()

	// run the gameboy until the breakpoint is reached
gameLoop:
	for {
		select {
		case <-ctx.Done():
			break gameLoop
		default:
			g.Frame()
			switch strat {
			case DebugBreakpoint:
				if g.CPU.DebugBreakpoint || g.Scheduler.Cycle() > 20*70240*60 {
					break gameLoop
				}
			case CycleBreakpoint:
				if int(g.Scheduler.Cycle()) > timeout*70240*60 || g.CPU.DebugBreakpoint { // cycle won't increase once breakpoint has been hit
					break gameLoop
				}
			}
		}
	}

	return g, nil
}

// TODO:
// - parse description from test roms (maybe)
// - model differentiation (DMG, CGB, SGB)
// - git clone to download test roms
// - blurb for each test suite (maybe)
// - tests have table entries for each test, with a link to the test rom, and a link to the expected image
// - palette compatibility dump
// - expected image output with actual image in README (with overlay)
// - individual test run
// - individual test suite table generation (not sure what I meant by this)
// - gameboy doctor
// - jsmoo tests
// - wilbertpol's tests
// - age tests
// - rtc tests
// - mealybug tests
// - failure reasons
// - ROMTest with TableEntry interface (for tests that provide a custom table entry)
