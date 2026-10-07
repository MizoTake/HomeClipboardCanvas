package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestTextLimitUsesDecodedContentRatherThanJSONEscaping(t *testing.T) {
	ts, client := newTestServer(t)
	data, _ := json.Marshal(map[string]any{"text": strings.Repeat("\\", 1<<20), "width": 320, "height": 240})
	r, _ := http.NewRequest("POST", ts.URL+"/api/items", bytes.NewReader(data))
	r.Header.Set("Origin", ts.URL)
	r.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 201 {
		t.Fatalf("valid 1MiB text rejected: %d", resp.StatusCode)
	}
}
