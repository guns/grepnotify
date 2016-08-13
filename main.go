/*
 * Copyright (c) 2016 Sung Pae <self@sungpae.com>
 * Distributed under the MIT license.
 * http://www.opensource.org/licenses/mit-license.php
 */

package main

import (
	"bufio"
	"fmt"
	"os"
	"path"
	"sync"
	"time"

	"github.com/jessevdk/go-flags"
)

const usagesummary = `[options] regexp summary-template message-template …

Scan stdin and notify on matching messages. Includes rate-limiting to prevent
notification floods.

Example:

  Notification on iptables events

  # iptables --new-chain DROPOUTPUT
  # iptables --append    DROPOUTPUT --jump LOG --log-prefix '[DROPOUTPUT] '
  # iptables --append    DROPOUTPUT --jump DROP
  # iptables --append    OUTPUT     --jump DROPOUTPUT

  $ dmesg --follow --notime | scanfnotify --delay 1000 \
      '^\[DROPOUTPUT\].*?OUT=(?P<out>\S*).*?.*?DST=(?P<dst>\S*).*?PROTO=(?P<proto>\S*).*?DPT=(?P<dpt>\S*)' \
      'DROPOUTPUT' \
      'to: ${dst} ${dpt}/${proto}\ndev: ${out}'

Multiple rules can be defined by supplying subsequent argument triplets.

See https://golang.org/pkg/regexp/#Regexp.Expand for documentation on
regexp templates.`

type options struct {
	Delay uint `short:"d" long:"delay" default:"0" description:"Polling delay (per replacement) in milliseconds"`
}

// func (opts *options) validate() error {
// 	switch {
// 	default:
// 		return nil
// 	}
// }

func abort(err error) {
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err.Error()) // About to exit, ignore
	}
	os.Exit(1)
}

func abortWithHelp(parser *flags.Parser) {
	parser.WriteHelp(os.Stderr)
	os.Exit(1)
}

func getopts(arguments []string) (parser *flags.Parser, opts *options, args []string) {
	opts = new(options)
	var err error

	parser = flags.NewNamedParser(path.Base(arguments[0]), flags.HelpFlag|flags.PassDoubleDash)
	parser.Usage = usagesummary

	if _, err = parser.AddGroup("Options", "", opts); err != nil {
		abort(err)
	}

	if args, err = parser.ParseArgs(arguments[1:]); err != nil {
		abort(err)
	}

	// if err = opts.validate(); err != nil {
	// 	abort(err)
	// }

	return parser, opts, args
}

func main() {
	parser, opts, args := getopts(os.Args)
	if len(args) == 0 {
		abortWithHelp(parser)
	}

	reps, err := parseReplacements(args, opts)
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
		i := i
		go func() {
			notifyReplacement(&reps[i], time.Duration(opts.Delay)*time.Millisecond)
			wg.Done()
		}()
	}

	wg.Wait()
}
