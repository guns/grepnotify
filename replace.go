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
	pat        regexp.Regexp
	fmt        string
	submatches int
	args       []interface{} // for fmt.Sprintf
	ch         chan notification
	busy       int32
}

var verbPattern = regexp.MustCompile(`%[^%]`)

func parseReplacements(args []string, opts *options) ([]replacement, error) {
	if len(args)&1 == 1 {
		return nil, errors.New("arguments must come in pairs")
	}

	n := len(args)
	reps := make([]replacement, n>>1)

	for i := 0; i < n; i += 2 {
		p, err := regexp.Compile(args[i])
		if err != nil {
			return nil, err
		}

		submatches := len(p.SubexpNames()) - 1
		verbs := len(verbPattern.FindAllString(args[i+1], -1))

		if submatches == 0 && verbs != 0 {
			return nil, fmt.Errorf("`%v` has no submatches, but `%v` contains format verbs", p, args[i+1])
		} else if verbs != submatches-1 {
			s := "es"
			if submatches == 1 {
				s = ""
			}
			v := "s"
			if verbs == 1 {
				v = ""
			}
			return nil, fmt.Errorf(
				"`%v` has %d submatch%s, but `%v` has %d verb%s, not %d",
				p, submatches, s, args[i+1], verbs, v, submatches-1,
			)
		}

		r := &reps[i>>1]
		r.pat = *p
		r.fmt = args[i+1]
		r.submatches = len(p.SubexpNames()) - 1
		r.args = make([]interface{}, r.submatches-1)
		r.ch = make(chan notification)
		if opts.Delay == 0 {
			r.busy = -1
		}
	}

	return reps, nil
}

func scanReplacements(reps []replacement, s *bufio.Scanner) {
	for s.Scan() {
		line := s.Text()
		var msg notification

		for i := range reps {
			r := &reps[i]

			// Try to avoid unnecessary work
			if r.busy != -1 && atomic.LoadInt32(&r.busy) == 1 {
				continue
			}

			if r.submatches == 0 {
				if !r.pat.MatchString(line) {
					continue
				}

				msg = notification{r.fmt, line}
			} else {
				matches := r.pat.FindStringSubmatch(line)
				if matches == nil {
					continue
				}

				for i, m := range matches[2:] {
					r.args[i] = m
				}

				msg = notification{matches[1], fmt.Sprintf(r.fmt, r.args...)}
			}

			if r.busy == -1 {
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
	if rep.busy == -1 {
		for line := range rep.ch {
			notify(line)
		}
	} else {
		for line := range rep.ch {
			atomic.StoreInt32(&rep.busy, 1)
			notify(line)
			time.Sleep(delay) // rate limiting
			atomic.StoreInt32(&rep.busy, 0)
		}
	}
}

func notify(n notification) {
	if err := exec.Command("notify-send", n.summary, n.body).Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
}
