package main

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"sync/atomic"
	"time"
)

type notification struct {
	summary string
	body    string
}

type replacement struct {
	pat         regexp.Regexp
	summaryTmpl string
	bodyTmpl    string
	ch          chan notification
	busy        int32
}

const (
	never = -1
	idle  = 0
	busy  = 1
)

func parseReplacements(args []string, opts *options) ([]replacement, error) {
	n := len(args)
	if n%3 != 0 {
		return nil, errors.New("arguments must be given in groups of three")
	}

	reps := make([]replacement, n/3)

	for i := 0; i < n; i += 3 {
		p, err := regexp.Compile(args[i])
		if err != nil {
			return nil, err
		}

		r := &reps[i/3]
		r.pat = *p // vet: okay to copy here then discard
		r.summaryTmpl = args[i+1]
		r.bodyTmpl = args[i+2]
		r.ch = make(chan notification)
		if opts.Delay == 0 {
			r.busy = never
		}
	}

	return reps, nil
}

func scanReplacements(reps []replacement, s *bufio.Scanner) {
	for s.Scan() {
		line := s.Text()

		for i := range reps {
			r := &reps[i]

			// Try to avoid unnecessary work
			if r.busy != never && atomic.LoadInt32(&r.busy) == busy {
				continue
			}

			matches := r.pat.FindStringSubmatchIndex(line)
			if matches == nil {
				continue
			}

			msg := notification{
				string(r.pat.ExpandString([]byte{}, r.summaryTmpl, line, matches)),
				string(r.pat.ExpandString([]byte{}, r.bodyTmpl, line, matches)),
			}

			if r.busy == never {
				r.ch <- msg
			} else {
				// Non-blocking
				select {
				case r.ch <- msg:
				default:
				}
			}

			break
		}
	}
}

func notifyReplacement(rep *replacement, delay time.Duration) {
	if rep.busy == never {
		for line := range rep.ch {
			go notify(line)
		}
	} else {
		for line := range rep.ch {
			atomic.StoreInt32(&rep.busy, busy)
			go notify(line)
			time.Sleep(delay) // rate limiting
			atomic.StoreInt32(&rep.busy, idle)
		}
	}
}

func notify(n notification) {
	if err := exec.Command("notify-send", n.summary, n.body).Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
}
