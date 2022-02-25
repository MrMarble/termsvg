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
	Duration      float32 `json:"duration,omitempty"`
	IdleTimeLimit float32 `json:"idle_time_limit,omitempty"`
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

func (r *Cast) fromJSON(file string) error {
	lines := strings.Split(file, "\n")
	if lines[0][0] == '{' {
		err := json.Unmarshal([]byte(lines[0]), &r.Header)
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

		r.Events = append(r.Events, event)
	}

	return nil
}

func (r *Cast) ToJSON() ([]byte, error) {
	header, err := json.Marshal(&r.Header)
	if err != nil {
		return nil, err
	}

	for i := range r.Events {
		header = append(header, '\n')

		js, err := json.Marshal(&r.Events[i])
		if err != nil {
			return nil, err
		}

		header = append(header, js...)
	}

	return header, nil
}

func (r *Cast) ToRelativeTime() {
	prev := 0.

	for i, frame := range r.Events {
		delay := frame.Time - prev
		prev = frame.Time
		r.Events[i].Time = delay
	}
}

func (r *Cast) CapRelativeTime(limit float64) {
	if limit > 0 {
		for i, frame := range r.Events {
			r.Events[i].Time = math.Min(frame.Time, limit)
		}
	}
}

func (r *Cast) ToAbsoluteTime() {
	time := 0.

	for i, frame := range r.Events {
		time += frame.Time
		r.Events[i].Time = time
	}
}

func (r *Cast) AdjustSpeed(speed float64) {
	for i := range r.Events {
		r.Events[i].Time /= speed
	}
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
