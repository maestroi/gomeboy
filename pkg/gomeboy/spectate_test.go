package gomeboy

import (
	"bytes"
	"image"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSpectatorServesCapturedFrame(t *testing.T) {
	e := newTestEmulator(t)
	defer e.Close()
	e.StepFrames(10)

	s := NewSpectator()
	if err := s.Capture(e); err != nil {
		t.Fatalf("Capture: %v", err)
	}

	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/frame.png")
	if err != nil {
		t.Fatalf("GET /frame.png: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "image/png" {
		t.Errorf("Content-Type = %q, want image/png", ct)
	}
	img, format, err := image.Decode(resp.Body)
	if err != nil {
		t.Fatalf("image.Decode: %v", err)
	}
	if format != "png" {
		t.Errorf("format = %q, want png", format)
	}
	b := img.Bounds()
	if b.Dx() != 160 || b.Dy() != 144 {
		t.Errorf("bounds = %v, want 160x144", b)
	}
}

func TestSpectatorBeforeCaptureIs503(t *testing.T) {
	s := NewSpectator()
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/frame.png")
	if err != nil {
		t.Fatalf("GET /frame.png: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", resp.StatusCode)
	}
}

func TestSpectatorIndexPageReferencesFramePNG(t *testing.T) {
	s := NewSpectator()
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("GET /: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if !bytes.Contains(body, []byte("/frame.png")) {
		t.Errorf("body does not contain /frame.png: %s", body)
	}
}
