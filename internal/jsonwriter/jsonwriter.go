package jsonwriter

import (
	"encoding/json"
	"io"
	"time"
)

type JSONWriter struct {
	delegate io.Writer
	level    string
}

func New(delegate io.Writer) *JSONWriter {
	return &JSONWriter{
		delegate: delegate,
		level:    "info",
	}
}

func NewWithLevel(delegate io.Writer, level string) *JSONWriter {
	return &JSONWriter{
		delegate: delegate,
		level:    level,
	}
}

func (j *JSONWriter) Write(b []byte) (int, error) {
	var js json.RawMessage
	if json.Unmarshal(b, &js) == nil {
		return j.delegate.Write(b)
	}

	entry := struct {
		Msg       string `json:"msg,omitempty"`
		Timestamp string `json:"ts,omitempty"`
		Level     string `json:"level,omitempty"`
	}{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Level:     j.level,
		Msg:       string(b),
	}

	enc := json.NewEncoder(j.delegate)
	if err := enc.Encode(&entry); err != nil {
		return 0, err
	}

	return len(b), nil
}
