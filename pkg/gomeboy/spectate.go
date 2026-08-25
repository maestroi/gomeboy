package gomeboy

import (
	"fmt"
	"net/http"
	"sync"
)

const spectatorPage = `<!doctype html>
<html>
<head>
<meta charset="utf-8">
<title>GomeBoy Spectator</title>
</head>
<body style="margin:0;background:#000">
<img src="/frame.png" id="f" style="width:100%;image-rendering:pixelated">
<script>setInterval(() => { document.getElementById('f').src = '/frame.png?t=' + Date.now() }, 200)</script>
</body>
</html>
`

// Spectator serves the current frame of an Emulator over HTTP for
// read-only viewing. It does not accept input. Call Capture to refresh
// what Handler serves after each StepFrame/StepFrames call.
type Spectator struct {
	mu  sync.RWMutex
	png []byte
}

// NewSpectator creates a Spectator with no frame yet captured.
func NewSpectator() *Spectator {
	return &Spectator{}
}

// Capture encodes e's current frame as PNG and stores it for serving.
// Call this after every StepFrame/StepFrames call you want visible to
// viewers — it does not happen automatically, so the caller controls
// the update rate (e.g. once per real frame, or throttled to 1-5/sec).
func (s *Spectator) Capture(e *Emulator) error {
	s.mu.Lock()
	data, err := e.PNG()
	if err == nil {
		s.png = data
	}
	s.mu.Unlock()
	return err
}

// Handler returns an http.Handler serving:
//   GET /frame.png  - the most recently captured frame as image/png
//   GET /            - a minimal auto-refreshing HTML page showing it
func (s *Spectator) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/frame.png", func(w http.ResponseWriter, r *http.Request) {
		s.mu.RLock()
		png := s.png
		s.mu.RUnlock()
		if png == nil {
			http.Error(w, "no frame captured yet", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		w.Write(png)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, spectatorPage)
	})
	return mux
}
