//go:build test

package tests

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
)

// skipKnownFailures is disabled under the "test" build tag: Test_Regressions
// relies on the Test_All subprocess exiting with status 1 (known failures
// must keep failing) and on the README table reporting them as failures.
var skipKnownFailures = false

func Test_All(t *testing.T) {
	testTable := testAllTable()

	// execute tests
	for _, top := range testTable.testSuites {
		suite := top
		t.Run(suite.name, func(t *testing.T) {
			t.Parallel()
			for _, collection := range suite.collections {
				col := collection
				t.Run(col.name, func(t *testing.T) {
					t.Parallel()
					col.Run(t)
				})
			}
		})
	}

	t.Cleanup(func() {
		// write markdown table to README.md
		f, err := os.Create("README.md")
		if err != nil {
			panic(err)
		}

		_, err = f.WriteString(testTable.CreateReadme())

		if err != nil {
			panic(err)
		}

		if err := f.Close(); err != nil {
			panic(err)
		}

		// update the test results table in the main readme
		b, err := os.ReadFile("../README.md")
		if err != nil {
			panic(err)
		}

		newResults := testTable.createTestResultsTable()

		b = findTableRE.ReplaceAll(b, []byte(newResults))
		b = progressRE.ReplaceAll(b, []byte(testTable.createProgressBar()))

		if err := os.WriteFile("../README.md", b, 0644); err != nil {
			panic(err)
		}
	})
}

type regressionTests map[string]int

func Test_Regressions(t *testing.T) {
	// load README from main branch
	req, err := http.Get("https://raw.githubusercontent.com/thelolagemann/gomeboy/main/tests/README.md")
	if err != nil {
		t.Error(err)
	}
	defer req.Body.Close()

	// read bytes
	b, err := io.ReadAll(req.Body)
	if err != nil {
		t.Error(err)
	}

	currentTests := parseTable(string(b))

	// jump to basepath
	if err := os.Chdir(basePath); err != nil {
		t.Error(err)
	}

	// read existing README to compare against to make sure file changed
	oldF, err := os.Open("README.md")
	if err != nil {
		panic(err)
	}
	oldB, err := io.ReadAll(oldF)
	if err != nil {
		panic(err)
	}

	// run test with exec (cheeky hack to avoid exit status 1 on failure)
	cmd := exec.Command("go", "test", "-tags", "test", "-v", "-run", "Test_All")
	var exitError *exec.ExitError
	var out strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); errors.As(err, &exitError) {
		if exitError.ExitCode() > 1 {
			t.Error(err)
		} else {
			fmt.Println(err, out.String())
		}
	} else {
		t.Error(err)
	}

	// load local README for comparison
	f, err := os.Open("README.md")
	if err != nil {
		t.Error(err)
	}
	defer f.Close()
	newB, err := io.ReadAll(f)
	if err != nil {
		t.Error(err)
	}

	if bytes.Equal(b, newB) {
		t.Error("no changes detected in README file", string(oldB), string(newB))
	}

	newTests := parseTable(string(newB))

	// check that each test suite either passes the same number, or a greater number of tests (TODO per test specificity)
	for suite, passed := range currentTests {
		t.Run(suite, func(t *testing.T) {
			if newTests[suite] < passed {
				t.Errorf("%s has a regression, %d -> %d", suite, passed, newTests[suite])
			}
		})
	}

	if t.Failed() {
		fmt.Println(string(oldB), string(newB))
	}

}

func parseTable(markdown string) regressionTests {
	matches := parseTableRE.FindAllStringSubmatch(markdown, -1)

	tests := make(regressionTests)

	for _, match := range matches {
		suite := match[1]
		passed, _ := strconv.Atoi(match[3])
		tests[suite] = passed
	}

	return tests
}
