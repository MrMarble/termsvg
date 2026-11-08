package asciicast

import (
	"encoding/json"
	"testing"
)

func TestEvent_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    Event
		wantErr bool
	}{
		{name: "output", in: `[1.5,"o","hi"]`, want: Event{Time: 1.5, EventType: Output, EventData: "hi"}},
		{name: "input", in: `[0,"i","\u001b"]`, want: Event{EventType: Input, EventData: "\x1b"}},
		{name: "not an array", in: `{"a":1}`, wantErr: true},
		{name: "too few elements", in: `[1,"o"]`, wantErr: true},
		{name: "too many elements", in: `[1,"o","x",4]`, wantErr: true},
		{name: "time not number", in: `["1","o","x"]`, wantErr: true},
		{name: "type not string", in: `[1,2,"x"]`, wantErr: true},
		{name: "data not string", in: `[1,"o",3]`, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got Event
			err := json.Unmarshal([]byte(tt.in), &got)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestEvent_MarshalJSON(t *testing.T) {
	e := Event{Time: 2.5, EventType: Output, EventData: "a\"b\n"}
	got, err := json.Marshal(&e)
	if err != nil {
		t.Fatal(err)
	}
	if want := `[2.5,"o","a\"b\n"]`; string(got) != want {
		t.Errorf("got %s, want %s", got, want)
	}
}
