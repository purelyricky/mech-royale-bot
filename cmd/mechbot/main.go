package main

import (
	"apexbot/internal/bot"
	"apexbot/internal/proto"
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"runtime"
)

// A dedicated reader drains stdin while a decision is running. Only the newest
// queued observation is acted upon; hello/end are never silently discarded.
func run(in io.Reader, out io.Writer) error {
	lines := make(chan []byte, 512)
	go func() {
		defer close(lines)
		s := bufio.NewScanner(in)
		s.Buffer(make([]byte, 65536), 4<<20)
		for s.Scan() {
			p := append([]byte(nil), s.Bytes()...)
			lines <- p
		}
		if err := s.Err(); err != nil {
			fmt.Fprintln(os.Stderr, "stdin:", err)
		}
	}()
	enc := json.NewEncoder(out)
	var brain *bot.Bot
	for raw := range lines {
		var latest *proto.Observation
		ended := false
		consume := func(p []byte) error {
			var head struct {
				Type string `json:"type"`
			}
			if err := json.Unmarshal(p, &head); err != nil {
				return nil
			}
			switch head.Type {
			case "hello":
				var h proto.Hello
				if err := json.Unmarshal(p, &h); err != nil {
					return err
				}
				if h.Protocol != "mechfight/1" {
					return fmt.Errorf("unsupported protocol %q", h.Protocol)
				}
				brain = bot.New(h)
				return enc.Encode(map[string]string{"type": "ready", "name": "last-light-go-v2"})
			case "obs":
				var o proto.Observation
				if err := json.Unmarshal(p, &o); err == nil {
					latest = &o
				}
			case "end":
				ended = true
			}
			return nil
		}
		if err := consume(raw); err != nil {
			return err
		}
	drain:
		for !ended {
			select {
			case p, ok := <-lines:
				if !ok {
					break drain
				}
				if err := consume(p); err != nil {
					return err
				}
			default:
				break drain
			}
		}
		if ended {
			return nil
		}
		if latest != nil && brain != nil {
			a := decide(brain, *latest)
			if err := enc.Encode(a); err != nil {
				return err
			}
		}
	}
	return nil
}
func decide(brain *bot.Bot, o proto.Observation) (a proto.Action) {
	a = proto.Action{Tick: o.Tick, Aim: o.Self.Aim}
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintln(os.Stderr, "decision recovered:", r)
		}
	}()
	return brain.Act(o)
}
func main() {
	runtime.GOMAXPROCS(1)
	if err := run(os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
