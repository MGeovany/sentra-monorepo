package cli

import (
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"time"
)

type spinner struct {
	enabled bool
	stop    chan struct{}
	done    chan struct{}
	msg     atomic.Value // string
}

func spinnerEnabled() bool {
	if !colorEnabled() {
		return false
	}
	v := strings.TrimSpace(os.Getenv("SENTRA_NO_SPINNER"))
	if v == "1" || strings.EqualFold(v, "true") {
		return false
	}
	return isTTY(os.Stdout)
}

func startSpinner(message string) *spinner {
	s := &spinner{enabled: spinnerEnabled(), stop: make(chan struct{}), done: make(chan struct{})}
	s.msg.Store(strings.TrimSpace(message))
	if !s.enabled {
		if strings.TrimSpace(message) != "" {
			infof("%s", message)
		}
		close(s.done)
		return s
	}

	go func() {
		defer close(s.done)
		frames := []string{"-", "\\", "|", "/"}
		i := 0
		for {
			select {
			case <-s.stop:
				// Clear line.
				_, _ = fmt.Fprint(os.Stdout, "\r\x1b[2K")
				return
			case <-time.After(90 * time.Millisecond):
				m, _ := s.msg.Load().(string)
				if strings.TrimSpace(m) == "" {
					m = "working..."
				}
				frame := frames[i%len(frames)]
				i++
				_, _ = fmt.Fprintf(os.Stdout, "\r%s", c(ansiDim, frame+" "+m))
			}
		}
	}()
	return s
}

func (s *spinner) Set(message string) {
	if s == nil {
		return
	}
	s.msg.Store(strings.TrimSpace(message))
}

func (s *spinner) StopSuccess(line string) {
	if s == nil {
		return
	}
	if s.enabled {
		close(s.stop)
		<-s.done
	}
	if strings.TrimSpace(line) != "" {
		successf("%s", line)
	}
}

func (s *spinner) StopInfo(line string) {
	if s == nil {
		return
	}
	if s.enabled {
		close(s.stop)
		<-s.done
	}
	if strings.TrimSpace(line) != "" {
		infof("%s", line)
	}
}
