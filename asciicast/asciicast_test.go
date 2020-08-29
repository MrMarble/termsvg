package asciicast

import "testing"

func TestReadRecords(t *testing.T) {
	record, err := ReadRecords("./test_data/test")
	if err != nil {
		t.Fatalf("Error reading: %v", err)
	}

	if record.Header.Version != 2 {
		t.Errorf("Expected version: %v, got: %v", 2, record.Header.Version)
	}
	if record.Header.Width != 213 {
		t.Errorf("Expected width: %v, got: %v", 213, record.Header.Width)
	}
	if record.Header.Height != 58 {
		t.Errorf("Expected height: %v, got: %v", 58, record.Header.Height)
	}
	if record.Header.Timestamp != 1598646467 {
		t.Errorf("Expected timestamp: %v, got: %v", 1598646467, record.Header.Timestamp)
	}
	if record.Header.Env.Term != "alacritty" {
		t.Errorf("Expected term: %v, got: %v", "alacritty", record.Header.Env.Term)
	}
}
