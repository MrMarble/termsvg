package asciicast

import (
	"encoding/json"
)

type eventType string

// Event is a 3-tuple encoded as JSON array.
type Event struct {
	Time      float64   `json:"time"`
	EventType eventType `json:"event-type"`
	EventData string    `json:"event-data"`
}

const (
	Input  eventType = "i" // Data read from stdin.
	Output eventType = "o" // Data writed to stdout.
)

// UnmarshalJSON reads json list as Event fields.
func (e *Event) UnmarshalJSON(data []byte) error {
	var v []interface{}
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}

	e.Time = v[0].(float64)
	e.EventType = eventType(v[1].(string))
	e.EventData = v[2].(string)

	return nil
}

// MarshalJSON reads json list as Event fields.
func (e *Event) MarshalJSON() ([]byte, error) {
	data := [...]interface{}{e.Time, string(e.EventType), e.EventData}

	v, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}

	return v, nil
}
