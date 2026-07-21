package trace

import (
	"encoding/json"
	"fmt"
	"io"
	"sync"
)

// JSONLines writes one stable Event JSON object per line.
type JSONLines struct {
	mutex   sync.Mutex
	encoder *json.Encoder
}

func NewJSONLines(writer io.Writer) (*JSONLines, error) {
	if writer == nil {
		return nil, fmt.Errorf("trace: JSON Lines writer is nil")
	}
	return &JSONLines{encoder: json.NewEncoder(writer)}, nil
}

func (j *JSONLines) Record(event Event) error {
	if j == nil || j.encoder == nil {
		return fmt.Errorf("trace: JSON Lines sink is nil")
	}
	j.mutex.Lock()
	defer j.mutex.Unlock()
	return j.encoder.Encode(event)
}
