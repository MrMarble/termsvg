// Package asciicast provides methods for working
// with asciinema's file format asciicast v2.
//
// Refer to the official documentation about asciicast v2 format here:
// https://github.com/asciinema/asciinema/blob/develop/doc/asciicast-v2.md
package asciicast

import (
	"encoding/json"
	"math"
	"os"
	"strings"
	"time"
)

// header is JSON-encoded object containing recording meta-data.
// fields with 'omitempty' are optional by asciicast v2 format
type header struct {
	Version       int     `json:"version"`
	Width         int     `json:"width"`
	Height        int     `json:"height"`
	Timestamp     int64   `json:"timestamp,omitempty"`
	Duration      float64 `json:"duration,omitempty"`
	IdleTimeLimit float64 `json:"idle_time_limit,omitempty"`
	Command       string  `json:"command,omitempty"`
	Title         string  `json:"string,omitempty"`
	Env           struct {
		Shell string `json:"SHELL,omitempty"`
		Term  string `json:"TERM,omitempty"`
	} `json:"env,omitempty"`
}

// Cast contains asciicast file data
type Cast struct {
	Header header
	Events []Event
}

// New will instantiate new Cast with basic medatada (version, timestamp and environment).
func New() *Cast {
	const version = 2

	cast := &Cast{
		Header: header{
			Version:   version,
			Timestamp: time.Now().Unix(),
		},
		Events: []Event{},
	}

	cast.Header.CaptureEnv()

	return cast
}

// CaptureEnv stores the environment variables 'shell' and 'term'.
func (h *header) CaptureEnv() {
	h.Env.Shell = os.Getenv("SHELL")
	h.Env.Term = os.Getenv("TERM")
}

// Marshal returns the JSON-like encoding of v.
func (c *Cast) Marshal() ([]byte, error) {
	header, err := json.Marshal(&c.Header)
	if err != nil {
		return nil, err
	}

	for i := range c.Events {
		header = append(header, '\n')

		js, err := json.Marshal(&c.Events[i])
		if err != nil {
			return nil, err
		}

		header = append(header, js...)
	}

	return header, nil
}

// Unmarshal parses the JSON-encoded data into a Cast struct.
func Unmarshal(data []byte) (*Cast, error) {
	var cast Cast

	err := cast.fromJSON(string(data))
	if err != nil {
		return nil, err
	}

	// Duration field isn't required as v2 documentation but is needed for exporting purposes.
	if cast.Header.Duration == 0 {
		cast.Header.Duration = cast.Events[len(cast.Events)-1].Time
	}

	return &cast, nil
}

// ToRelativeTime converts event time to the difference between each event.
func (c *Cast) ToRelativeTime() {
	prev := 0.

	for i, frame := range c.Events {
		delay := frame.Time - prev
		prev = frame.Time
		c.Events[i].Time = delay
	}
}

// CapRelativeTime limits the amount of time between each event
func (c *Cast) CapRelativeTime(limit float64) {
	if limit > 0 {
		for i, frame := range c.Events {
			c.Events[i].Time = math.Min(frame.Time, limit)
		}
	}
}

// ToAbsoluteTime converts event time to the absolute difference from the start.
// This is the default time format.
func (c *Cast) ToAbsoluteTime() {
	time := 0.

	for i, frame := range c.Events {
		time += frame.Time
		c.Events[i].Time = time
	}
}

// AdjustSpeed changes the time of each event.
// Slower < 1.0 > Faster.
func (c *Cast) AdjustSpeed(speed float64) {
	for i := range c.Events {
		c.Events[i].Time /= speed
	}
}

// Compress chains together events with the same time.
func (c *Cast) Compress() {
	var events []Event

	for i, event := range c.Events {
		if i == 0 {
			events = append(events, event)
			continue
		} else {
			if event.Time == events[len(events)-1].Time {
				events[len(events)-1].EventData += event.EventData
			} else {
				events = append(events, event)
			}
		}
	}

	c.Events = events
}

// Asciicast format is not valid JSON so json.Unmarshal returns an error.
// This function parses the file line by line to circumvent that.
func (c *Cast) fromJSON(data string) error {
	lines := strings.Split(data, "\n")
	if lines[0][0] == '{' {
		err := json.Unmarshal([]byte(lines[0]), &c.Header)
		if err != nil {
			return err
		}

		lines = lines[1:]
	}

	for _, line := range lines {
		var event Event

		err := json.Unmarshal([]byte(line), &event)
		if err != nil {
			return err
		}

		c.Events = append(c.Events, event)
	}

	return nil
}
