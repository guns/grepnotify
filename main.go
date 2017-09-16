/*
 * Copyright (c) 2016-2017 Sung Pae <self@sungpae.com>
 * Distributed under the MIT license.
 * http://www.opensource.org/licenses/mit-license.php
 */

package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"os"
	"sync"
)

const usage = `Usage:
  grepnotify [options] regexp summary-template message-template …

Scan stdin and notify on matching messages. Includes rate-limiting to prevent
notification floods.

Example:

  Notification on iptables events

  # iptables --new-chain DROPOUTPUT
  # iptables --append    DROPOUTPUT --jump LOG --log-prefix '[DROPOUTPUT] '
  # iptables --append    DROPOUTPUT --jump DROP
  # iptables --append    OUTPUT     --jump DROPOUTPUT

  $ dmesg --follow --notime | grepnotify -delay 1s \
      '^\[DROPOUTPUT\].*?OUT=(?P<out>\S*).*?.*?DST=(?P<dst>\S*).*?PROTO=(?P<proto>\S*).*?DPT=(?P<dpt>\S*)' \
      'DROPOUTPUT' \
      'to: ${dst} ${dpt}/${proto}\ndev: ${out}'

Multiple rules can be defined by supplying subsequent argument triplets.

See https://golang.org/pkg/regexp/#Regexp.Expand for documentation on
regexp templates.

Options:`

func abort(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
	}
	os.Exit(1)
}

func main() {
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, usage)
		flag.PrintDefaults()
	}
	delay := flag.Duration("delay", 0, "Polling delay per replacement")
	flag.Parse()

	if flag.NArg() == 0 {
		abort(errors.New("no arguments given"))
	}

	reps, err := parseReplacements(flag.Args(), *delay)
	if err != nil {
		abort(err)
	}

	s := bufio.NewScanner(os.Stdin)
	wg := sync.WaitGroup{}
	wg.Add(1 + len(reps))

	go func() {
		scanReplacements(reps, s)
		for i := range reps {
			close(reps[i].ch)
		}
		wg.Done()
	}()

	for i := range reps {
		go func(i int) {
			notifyReplacement(&reps[i], *delay)
			wg.Done()
		}(i)
	}

	wg.Wait()
}
