package main

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"regexp"
	"sync/atomic"
	"time"
)

type replacement struct {
	pat  regexp.Regexp
	fmt  string
	args []interface{} // for fmt.Sprintf
	ch   chan string
	busy int32
}

var verb = regexp.MustCompile(`%[^%]`)

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
		} else if len(p.SubexpNames())-1 != len(verb.FindAllString(args[i+1], -1)) {
			return nil, fmt.Errorf("number of submatches in `%v` does not match number of verbs in `%v`", p, args[i+1])
		}

		r := &reps[i>>1]
		r.pat = *p
		r.fmt = args[i+1]
		r.args = make([]interface{}, len(p.SubexpNames())-1)
		r.ch = make(chan string)
		if opts.Delay == 0 {
			r.busy = -1
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
			if r.busy != -1 && atomic.LoadInt32(&r.busy) == 1 {
				fmt.Fprintln(os.Stderr, "AVOIDING WORK!")
				continue
			}

			matches := r.pat.FindStringSubmatch(line)
			if matches == nil {
				continue
			}

			for i, m := range matches[1:] {
				r.args[i] = m
			}

			msg := fmt.Sprintf(r.fmt, r.args...)

			if r.busy == -1 {
				r.ch <- msg
			} else {
				// Non-blocking
				select {
				case r.ch <- msg:
				default:
					fmt.Fprintln(os.Stderr, "NOT BLOCKING!")
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

func notify(s string) {
	fmt.Println(s)
}
