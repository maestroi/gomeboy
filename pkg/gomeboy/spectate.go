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
<style>
  html,body { margin:0; height:100%; background:#111; color:#888;
              font:14px system-ui,sans-serif; }
  body { display:flex; align-items:center; justify-content:center; }
  /* The Game Boy screen is 160x144. Scale it in whole pixels and cap the
     size, so it never stretches to the width of the browser window. */
  #f { display:none; width:640px; height:576px; max-width:100vw;
       max-height:100vh; object-fit:contain; image-rendering:pixelated; }
</style>
</head>
<body>
<p id="w">waiting for the first frame...</p>
<img id="f" alt="">
<script>
const img = document.getElementById('f'), wait = document.getElementById('w');
let inFlight = false;
async function tick() {
  if (inFlight) return;
  inFlight = true;
  try {
    const r = await fetch('/frame.png', { cache: 'no-store' });
    if (r.ok) {
      const url = URL.createObjectURL(await r.blob());
      const old = img.src;
      img.src = url;
      if (old.startsWith('blob:')) URL.revokeObjectURL(old);
      img.style.display = 'block';
      wait.style.display = 'none';
    }
  } catch (e) { /* server gone; keep showing the last frame */ }
  finally { inFlight = false; }
}
setInterval(tick, 100);
tick();
</script>
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
