package asciicast

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"strings"
)

// Header  ...
type Header struct {
	Version       int8    `json:"version"`
	Width         int32   `json:"width"`
	Height        int32   `json:"height"`
	Timestamp     int32   `json:"timestamp"`
	Duration      float32 `json:"duration"`
	IdleTimeLimit float32 `json:"idle-time-limit,omitempty"`
	Command       string  `json:"command,omitempty"`
	Title         string  `json:"string"`
	Env           struct {
		Shell string `json:"SHELL"`
		Term  string `json:"TERM"`
	} `json:"env"`
}

// Event ...
type Event struct {
	Time      float64 `json:"time"`
	EventType string  `json:"event-type"`
	EventData string  `json:"event-data"`
}

func (e *Event) UnmarshalJSON(data []byte) error {
	var v []interface{}
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	e.Time = float64(v[0].(float64))
	e.EventType = string(v[1].(string))
	e.EventData = string(v[2].(string))
	return nil
}

// Record ...
type Record struct {
	Header Header
	Events []Event
}

func (r *Record) fromJSON(file string) {
	lines := strings.Split(file, "\n")
	if lines[0][0] == '{' {
		json.Unmarshal([]byte(lines[0]), &r.Header)
		lines = lines[1:]
	}
	for _, line := range lines {
		var event Event
		json.Unmarshal([]byte(line), &event)
		r.Events = append(r.Events, event)
	}
}

func escapeBytes(file []byte) []byte {
	return bytes.ReplaceAll(file, []byte("\\"), []byte("\\\\"))
}

// ReadRecords ...
func ReadRecords(filename string) (*Record, error) {
	file, err := ioutil.ReadFile(filename)
	if err == nil {
		var record Record
		record.fromJSON(string(escapeBytes(file)))
		return &record, nil
	}
	return nil, err
}
