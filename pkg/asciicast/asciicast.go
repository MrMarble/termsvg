package asciicast

import (
	"encoding/json"
	"io/ioutil"
	"math"
	"os"
	"strings"
	"time"
)

// Header is JSON-encoded object containing recording meta-data.
type Header struct {
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

// Cast ...
type Cast struct {
	Header Header
	Events []Event
}

func (h *Header) CaptureEnv() {
	h.Env.Shell = os.Getenv("SHELL")
	h.Env.Term = os.Getenv("TERM")
}

func (c *Cast) fromJSON(file string) error {
	lines := strings.Split(file, "\n")
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

func (c *Cast) ToJSON() ([]byte, error) {
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

func (c *Cast) ToRelativeTime() {
	prev := 0.

	for i, frame := range c.Events {
		delay := frame.Time - prev
		prev = frame.Time
		c.Events[i].Time = delay
	}
}

func (c *Cast) CapRelativeTime(limit float64) {
	if limit > 0 {
		for i, frame := range c.Events {
			c.Events[i].Time = math.Min(frame.Time, limit)
		}
	}
}

func (c *Cast) ToAbsoluteTime() {
	time := 0.

	for i, frame := range c.Events {
		time += frame.Time
		c.Events[i].Time = time
	}
}

func (c *Cast) AdjustSpeed(speed float64) {
	for i := range c.Events {
		c.Events[i].Time /= speed
	}
}

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

// ReadRecords ...
func ReadRecords(filename string) (*Cast, error) {
	file, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	var record Cast

	err = record.fromJSON(string(file))
	if err != nil {
		return nil, err
	}

	if record.Header.Duration == 0 {
		record.Header.Duration = record.Events[len(record.Events)-1].Time
	}

	return &record, nil
}

func NewRecord() *Cast {
	const version = 2

	cast := &Cast{
		Header: Header{
			Version:   version,
			Timestamp: time.Now().Unix(),
		},
		Events: []Event{},
	}

	cast.Header.CaptureEnv()

	return cast
}
