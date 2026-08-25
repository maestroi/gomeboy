package tests

import (
	"fmt"
	"github.com/thelolagemann/gomeboy/internal/gameboy"
	"github.com/thelolagemann/gomeboy/internal/io"
	"image/png"
	"math/rand/v2"
	"os"
	"testing"
	"time"
)

type inputTest struct {
	expectedImagePath string
	*basicTest
}

func (iT *inputTest) Run(t *testing.T) {
	iT.passed = true
	t.Run(iT.name, func(t *testing.T) {
		skipKnownFailure(t, iT.name)

		// attempt runs one full pass of the test; it is retried up to 5
		// times to absorb flakiness
		attempt := func() error {
			// create a new gameboy
			gb := gameboy.NewGameBoy(gameboy.AsModel(iT.model))
			if err := gb.LoadROM(iT.romPath); err != nil {
				return err
			}

			// run a second worth of frames for setup
			for i := 0; i < 55; i++ {
				gb.Frame()
			}

			var testFinished = false
			go func() {
				for i := io.ButtonA; i <= io.ButtonDown; i++ {
					for !gb.Running() {
					}
					for i := 0; i < rand.IntN(512); i++ {
					} // burn a random amount of time
					gb.Bus.Press(i)

					time.Sleep(time.Millisecond * 20)
					for !gb.Running() {
					}
					gb.Bus.Release(i)
					time.Sleep(time.Millisecond * 10)
				}
				testFinished = true
			}()
			for !testFinished {
				// get the next frame
				gb.Frame()
				time.Sleep(time.Millisecond * 5) // give some time for the input handler
			}
			for i := 0; i < 60*10; i++ {
				gb.Frame()
			}

			diff, diffImg, err := compareImage(iT.expectedImagePath, gb)
			if err != nil {
				iT.passed = false
				t.Fatal(err)
			}

			if diff > 0 {
				iT.passed = false

				// write output image to disk
				if err := os.MkdirAll("results", 0o755); err != nil {
					return err
				}
				outFile, err := os.Create(fmt.Sprintf("results/%s_output.png", iT.name))
				if err != nil {
					return err
				}
				if err := png.Encode(outFile, diffImg); err != nil {
					outFile.Close()
					return err
				}
				outFile.Close()
				return fmt.Errorf("images are different: %d", diff) // TODO percentage
			}
			iT.passed = true // a successful retry clears the flag a failed attempt set
			return nil
		}

		for i := 0; i < 5; i++ {
			if err := attempt(); err == nil {
				return // Test passed
			}
		}
		t.Fatalf("Test failed after 5 attempts")
	})
}

type testInput struct {
	// the button to press
	button io.Button
	// the frame to press the button
	atEmulatedCycle uint64
}
