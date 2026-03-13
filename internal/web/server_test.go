package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/masterphelps/flowboy/internal/config"
	"github.com/masterphelps/flowboy/internal/engine"
)

// newTestServer creates a Server backed by a real engine with no exporter
// and an in-memory config (no disk persistence).
func newTestServer(t *testing.T) (*Server, *httptest.Server) {
	t.Helper()
	eng := engine.New()
	cfg := &config.Config{}
	srv := NewServer(eng, nil, cfg, "", 0)
	ts := httptest.NewServer(srv)
	return srv, ts
}

// ---------- Machine tests ----------

func TestListMachinesEmpty(t *testing.T) {
	_, ts := newTestServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/machines")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var machines []machineResponse
	if err := json.NewDecoder(resp.Body).Decode(&machines); err != nil {
		t.Fatal(err)
	}
	if len(machines) != 0 {
		t.Fatalf("expected 0 machines, got %d", len(machines))
	}
}

func TestCreateMachine(t *testing.T) {
	_, ts := newTestServer(t)
	defer ts.Close()

	body := `{"name":"web-01","ip":"10.0.1.10","mask":24}`
	resp, err := http.Post(ts.URL+"/api/machines", "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}

	var m machineResponse
	if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
		t.Fatal(err)
	}
	if m.Name != "web-01" {
		t.Fatalf("expected name web-01, got %s", m.Name)
	}
	if m.IP != "10.0.1.10" {
		t.Fatalf("expected IP 10.0.1.10, got %s", m.IP)
	}
	if m.Mask != 24 {
		t.Fatalf("expected mask 24, got %d", m.Mask)
	}

	// Verify it appears in list.
	resp2, err := http.Get(ts.URL + "/api/machines")
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()

	var machines []machineResponse
	if err := json.NewDecoder(resp2.Body).Decode(&machines); err != nil {
		t.Fatal(err)
	}
	if len(machines) != 1 {
		t.Fatalf("expected 1 machine, got %d", len(machines))
	}
}

func TestDeleteMachine(t *testing.T) {
	_, ts := newTestServer(t)
	defer ts.Close()

	// Create machine first.
	body := `{"name":"db-01","ip":"10.0.2.20","mask":24}`
	resp, err := http.Post(ts.URL+"/api/machines", "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	// Delete it.
	req, _ := http.NewRequest(http.MethodDelete, ts.URL+"/api/machines/db-01", nil)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", resp.StatusCode)
	}

	// Verify it's gone.
	resp, err = http.Get(ts.URL + "/api/machines")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	var machines []machineResponse
	if err := json.NewDecoder(resp.Body).Decode(&machines); err != nil {
		t.Fatal(err)
	}
	if len(machines) != 0 {
		t.Fatalf("expected 0 machines after delete, got %d", len(machines))
	}
}

// ---------- Flow tests ----------

func TestListFlowsEmpty(t *testing.T) {
	_, ts := newTestServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/flows")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var flows []flowResponse
	if err := json.NewDecoder(resp.Body).Decode(&flows); err != nil {
		t.Fatal(err)
	}
	if len(flows) != 0 {
		t.Fatalf("expected 0 flows, got %d", len(flows))
	}
}

func TestCreateFlow(t *testing.T) {
	_, ts := newTestServer(t)
	defer ts.Close()

	// First add machines that the flow references.
	for _, m := range []string{
		`{"name":"src-01","ip":"10.0.1.10","mask":24}`,
		`{"name":"dst-01","ip":"10.0.2.20","mask":24}`,
	} {
		resp, err := http.Post(ts.URL+"/api/machines", "application/json", bytes.NewBufferString(m))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
	}

	flowBody := `{
		"name": "web-traffic",
		"source": "src-01",
		"source_port": 443,
		"destination": "dst-01",
		"destination_port": 8080,
		"protocol": "TCP",
		"rate": "10Mbps",
		"enabled": true
	}`
	resp, err := http.Post(ts.URL+"/api/flows", "application/json", bytes.NewBufferString(flowBody))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}

	var f flowResponse
	if err := json.NewDecoder(resp.Body).Decode(&f); err != nil {
		t.Fatal(err)
	}
	if f.Name != "web-traffic" {
		t.Fatalf("expected name web-traffic, got %s", f.Name)
	}
	if f.Source != "src-01" {
		t.Fatalf("expected source src-01, got %s", f.Source)
	}

	// Verify it appears in list.
	resp2, err := http.Get(ts.URL + "/api/flows")
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()

	var flows []flowResponse
	if err := json.NewDecoder(resp2.Body).Decode(&flows); err != nil {
		t.Fatal(err)
	}
	if len(flows) != 1 {
		t.Fatalf("expected 1 flow, got %d", len(flows))
	}
}

// ---------- Engine tests ----------

func TestEngineStatus(t *testing.T) {
	_, ts := newTestServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/engine/status")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var status engineStatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		t.Fatal(err)
	}
	if status.Running {
		t.Fatal("expected engine not running initially")
	}
	if status.FlowCount != 0 {
		t.Fatalf("expected 0 flows, got %d", status.FlowCount)
	}
	if status.Uptime == "" {
		t.Fatal("expected non-empty uptime")
	}
}

func TestEngineStartStop(t *testing.T) {
	_, ts := newTestServer(t)
	defer ts.Close()

	// Start engine.
	resp, err := http.Post(ts.URL+"/api/engine/start", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for start, got %d", resp.StatusCode)
	}

	// Check running.
	resp, err = http.Get(ts.URL + "/api/engine/status")
	if err != nil {
		t.Fatal(err)
	}
	var status engineStatusResponse
	json.NewDecoder(resp.Body).Decode(&status)
	resp.Body.Close()
	if !status.Running {
		t.Fatal("expected engine running after start")
	}

	// Stop engine.
	resp, err = http.Post(ts.URL+"/api/engine/stop", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for stop, got %d", resp.StatusCode)
	}

	// Check stopped.
	resp, err = http.Get(ts.URL + "/api/engine/status")
	if err != nil {
		t.Fatal(err)
	}
	json.NewDecoder(resp.Body).Decode(&status)
	resp.Body.Close()
	if status.Running {
		t.Fatal("expected engine stopped after stop")
	}
}

// ---------- Segments tests ----------

func TestSegmentsEmpty(t *testing.T) {
	_, ts := newTestServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/segments")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var segments []segmentResponse
	if err := json.NewDecoder(resp.Body).Decode(&segments); err != nil {
		t.Fatal(err)
	}
	if len(segments) != 0 {
		t.Fatalf("expected 0 segments, got %d", len(segments))
	}
}

func TestSegmentsWithMachines(t *testing.T) {
	_, ts := newTestServer(t)
	defer ts.Close()

	// Add machines in the same /24 subnet.
	for _, m := range []string{
		`{"name":"a","ip":"10.0.1.10","mask":24}`,
		`{"name":"b","ip":"10.0.1.20","mask":24}`,
		`{"name":"c","ip":"10.0.2.30","mask":24}`,
	} {
		resp, err := http.Post(ts.URL+"/api/machines", "application/json", bytes.NewBufferString(m))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
	}

	resp, err := http.Get(ts.URL + "/api/segments")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	var segments []segmentResponse
	if err := json.NewDecoder(resp.Body).Decode(&segments); err != nil {
		t.Fatal(err)
	}
	// Should have 2 segments: 10.0.1.0/24 (2 machines) and 10.0.2.0/24 (1 machine).
	if len(segments) != 2 {
		t.Fatalf("expected 2 segments, got %d", len(segments))
	}
}

// ---------- CORS test ----------

func TestCORSHeaders(t *testing.T) {
	_, ts := newTestServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/machines")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	origin := resp.Header.Get("Access-Control-Allow-Origin")
	if origin != "*" {
		t.Fatalf("expected CORS origin *, got %q", origin)
	}
}

// ---------- Collectors tests ----------

func TestCollectors(t *testing.T) {
	_, ts := newTestServer(t)
	defer ts.Close()

	// List empty.
	resp, err := http.Get(ts.URL + "/api/collectors")
	if err != nil {
		t.Fatal(err)
	}
	var collectors []json.RawMessage
	json.NewDecoder(resp.Body).Decode(&collectors)
	resp.Body.Close()
	if len(collectors) != 0 {
		t.Fatalf("expected 0 collectors, got %d", len(collectors))
	}

	// Add collector.
	body := `{"name":"elk","address":"10.0.0.5:9995","version":"v9"}`
	resp, err = http.Post(ts.URL+"/api/collectors", "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Verify it's there.
	resp, err = http.Get(ts.URL + "/api/collectors")
	if err != nil {
		t.Fatal(err)
	}
	json.NewDecoder(resp.Body).Decode(&collectors)
	resp.Body.Close()
	if len(collectors) != 1 {
		t.Fatalf("expected 1 collector, got %d", len(collectors))
	}

	// Delete it.
	req, _ := http.NewRequest(http.MethodDelete, ts.URL+"/api/collectors/elk", nil)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", resp.StatusCode)
	}
}

// ---------- OPTIONS preflight test ----------

func TestOptionsPreflightReturns200(t *testing.T) {
	_, ts := newTestServer(t)
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodOptions, ts.URL+"/api/machines", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for OPTIONS, got %d", resp.StatusCode)
	}
}
