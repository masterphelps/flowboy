package web

import (
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/masterphelps/flowboy/internal/anomaly"
	"github.com/masterphelps/flowboy/internal/config"
	"github.com/masterphelps/flowboy/internal/engine"
)

// ---------- Machines ----------

func (s *Server) handleMachines(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.listMachines(w, r)
	case http.MethodPost:
		s.createMachine(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleMachineByName(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/api/machines/")
	if name == "" {
		http.Error(w, "machine name required", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodPut:
		s.updateMachine(w, r, name)
	case http.MethodDelete:
		s.deleteMachine(w, r, name)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

type machineRequest struct {
	Name string `json:"name"`
	IP   string `json:"ip"`
	Mask int    `json:"mask"`
}

type machineResponse struct {
	Name string `json:"name"`
	IP   string `json:"ip"`
	Mask int    `json:"mask"`
}

func (s *Server) listMachines(w http.ResponseWriter, _ *http.Request) {
	machines := s.engine.Machines()
	resp := make([]machineResponse, 0, len(machines))
	for _, m := range machines {
		ones, _ := m.Mask.Size()
		resp = append(resp, machineResponse{
			Name: m.Name,
			IP:   m.IP.String(),
			Mask: ones,
		})
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) createMachine(w http.ResponseWriter, r *http.Request) {
	var req machineRequest
	if err := readJSON(r, &req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.Name == "" || req.IP == "" {
		http.Error(w, "name and ip are required", http.StatusBadRequest)
		return
	}

	mc := config.MachineConfig{Name: req.Name, IP: req.IP, Mask: req.Mask}
	m, err := mc.ToMachine()
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	s.engine.AddMachine(m)

	// Update persistent config.
	s.config.Machines = append(s.config.Machines, mc)
	s.saveConfig()

	writeJSON(w, http.StatusCreated, machineResponse{
		Name: m.Name,
		IP:   m.IP.String(),
		Mask: req.Mask,
	})
}

func (s *Server) updateMachine(w http.ResponseWriter, r *http.Request, oldName string) {
	var req machineRequest
	if err := readJSON(r, &req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.Name == "" || req.IP == "" {
		http.Error(w, "name and ip are required", http.StatusBadRequest)
		return
	}

	mc := config.MachineConfig{Name: req.Name, IP: req.IP, Mask: req.Mask}
	m, err := mc.ToMachine()
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	s.engine.UpdateMachine(oldName, m)

	// Update persistent config.
	for i, existing := range s.config.Machines {
		if existing.Name == oldName {
			s.config.Machines[i] = mc
			s.saveConfig()
			writeJSON(w, http.StatusOK, machineResponse{Name: m.Name, IP: m.IP.String(), Mask: req.Mask})
			return
		}
	}
	// Not found in config — add it
	s.config.Machines = append(s.config.Machines, mc)
	s.saveConfig()
	writeJSON(w, http.StatusOK, machineResponse{Name: m.Name, IP: m.IP.String(), Mask: req.Mask})
}

func (s *Server) deleteMachine(w http.ResponseWriter, _ *http.Request, name string) {
	// Block delete if any flow references this machine.
	for _, fc := range s.config.Flows {
		if fc.Source == name || fc.Destination == name {
			http.Error(w, "cannot delete machine \""+name+"\": referenced by flow \""+fc.Name+"\"", http.StatusConflict)
			return
		}
	}

	s.engine.RemoveMachine(name)

	// Remove from persistent config.
	filtered := s.config.Machines[:0]
	for _, mc := range s.config.Machines {
		if mc.Name != name {
			filtered = append(filtered, mc)
		}
	}
	s.config.Machines = filtered
	s.saveConfig()

	w.WriteHeader(http.StatusNoContent)
}

// ---------- Flows ----------

func (s *Server) handleFlows(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.listFlows(w, r)
	case http.MethodPost:
		s.createFlow(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleFlowByName(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/flows/")
	if rest == "" {
		http.Error(w, "flow name required", http.StatusBadRequest)
		return
	}

	parts := strings.SplitN(rest, "/", 2)
	name := parts[0]
	action := ""
	if len(parts) == 2 {
		action = parts[1]
	}

	switch {
	case action == "start" && r.Method == http.MethodPost:
		s.startFlow(w, r, name)
	case action == "stop" && r.Method == http.MethodPost:
		s.stopFlow(w, r, name)
	case action == "" && r.Method == http.MethodPut:
		s.updateFlow(w, r, name)
	case action == "" && r.Method == http.MethodDelete:
		s.deleteFlow(w, r, name)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

type flowResponse struct {
	Name        string `json:"name"`
	Source      string `json:"source"`
	SourcePort  uint16 `json:"source_port"`
	Destination string `json:"destination"`
	DestPort    uint16 `json:"destination_port"`
	Protocol    string `json:"protocol"`
	Rate        string `json:"rate"`
	AppID       uint32 `json:"app_id,omitempty"`
	Enabled     bool   `json:"enabled"`
}

func flowToResponse(f config.Flow) flowResponse {
	return flowResponse{
		Name:        f.Name,
		Source:      f.SourceName,
		SourcePort:  f.SourcePort,
		Destination: f.DestName,
		DestPort:    f.DestPort,
		Protocol:    f.Protocol,
		Rate:        f.Rate,
		AppID:       f.AppID,
		Enabled:     f.Enabled,
	}
}

func (s *Server) listFlows(w http.ResponseWriter, _ *http.Request) {
	flows := s.engine.Flows()
	resp := make([]flowResponse, 0, len(flows))
	for _, f := range flows {
		resp = append(resp, flowToResponse(f))
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) createFlow(w http.ResponseWriter, r *http.Request) {
	var fc config.FlowConfig
	if err := readJSON(r, &fc); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	f, err := fc.ToFlow()
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := s.engine.AddFlow(f); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Update persistent config.
	s.config.Flows = append(s.config.Flows, fc)
	s.saveConfig()

	writeJSON(w, http.StatusCreated, flowToResponse(f))
}

func (s *Server) updateFlow(w http.ResponseWriter, r *http.Request, name string) {
	_ = s.engine.RemoveFlow(name)

	var fc config.FlowConfig
	if err := readJSON(r, &fc); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	fc.Name = name
	f, err := fc.ToFlow()
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := s.engine.AddFlow(f); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Update persistent config.
	for i, existing := range s.config.Flows {
		if existing.Name == name {
			s.config.Flows[i] = fc
			s.saveConfig()
			writeJSON(w, http.StatusOK, flowToResponse(f))
			return
		}
	}
	s.config.Flows = append(s.config.Flows, fc)
	s.saveConfig()

	writeJSON(w, http.StatusOK, flowToResponse(f))
}

func (s *Server) deleteFlow(w http.ResponseWriter, _ *http.Request, name string) {
	if err := s.engine.RemoveFlow(name); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	filtered := s.config.Flows[:0]
	for _, fc := range s.config.Flows {
		if fc.Name != name {
			filtered = append(filtered, fc)
		}
	}
	s.config.Flows = filtered
	s.saveConfig()

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) startFlow(w http.ResponseWriter, _ *http.Request, name string) {
	flows := s.engine.Flows()
	for _, f := range flows {
		if f.Name == name {
			_ = s.engine.RemoveFlow(name)
			f.Enabled = true
			if err := s.engine.AddFlow(f); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			writeJSON(w, http.StatusOK, map[string]string{"status": "started"})
			return
		}
	}
	http.Error(w, "flow not found", http.StatusNotFound)
}

func (s *Server) stopFlow(w http.ResponseWriter, _ *http.Request, name string) {
	flows := s.engine.Flows()
	for _, f := range flows {
		if f.Name == name {
			_ = s.engine.RemoveFlow(name)
			f.Enabled = false
			if err := s.engine.AddFlow(f); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			writeJSON(w, http.StatusOK, map[string]string{"status": "stopped"})
			return
		}
	}
	http.Error(w, "flow not found", http.StatusNotFound)
}

// ---------- Collectors ----------

func (s *Server) handleCollectors(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.listCollectors(w, r)
	case http.MethodPost:
		s.addCollector(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleCollectorByName(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/api/collectors/")
	if name == "" {
		http.Error(w, "collector name required", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodDelete:
		s.removeCollector(w, r, name)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

type collectorResponse struct {
	Name    string `json:"name"`
	Address string `json:"address"`
	Version string `json:"version"`
}

func (s *Server) listCollectors(w http.ResponseWriter, _ *http.Request) {
	resp := make([]collectorResponse, 0, len(s.config.Collectors))
	for _, c := range s.config.Collectors {
		resp = append(resp, collectorResponse{
			Name:    c.Name,
			Address: c.Address,
			Version: c.Version,
		})
	}

	type collectorWithStats struct {
		collectorResponse
		PacketsSent uint64 `json:"packets_sent"`
		BytesSent   uint64 `json:"bytes_sent"`
		Errors      uint64 `json:"errors"`
	}

	var result []collectorWithStats
	var exporterStats map[string]*engine.ExporterStats
	if s.exporter != nil {
		exporterStats = s.exporter.GetStats()
	}

	for _, cr := range resp {
		cs := collectorWithStats{collectorResponse: cr}
		if exporterStats != nil {
			if st, ok := exporterStats[cr.Name]; ok {
				cs.PacketsSent = st.PacketsSent
				cs.BytesSent = st.BytesSent
				cs.Errors = st.Errors
			}
		}
		result = append(result, cs)
	}

	writeJSON(w, http.StatusOK, result)
}

func (s *Server) addCollector(w http.ResponseWriter, r *http.Request) {
	var c config.Collector
	if err := readJSON(r, &c); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if c.Name == "" || c.Address == "" {
		http.Error(w, "name and address are required", http.StatusBadRequest)
		return
	}
	if c.Version == "" {
		c.Version = "v9"
	}

	s.config.Collectors = append(s.config.Collectors, c)
	s.saveConfig()

	writeJSON(w, http.StatusCreated, collectorResponse{
		Name:    c.Name,
		Address: c.Address,
		Version: c.Version,
	})
}

func (s *Server) removeCollector(w http.ResponseWriter, _ *http.Request, name string) {
	filtered := s.config.Collectors[:0]
	for _, c := range s.config.Collectors {
		if c.Name != name {
			filtered = append(filtered, c)
		}
	}
	s.config.Collectors = filtered
	s.saveConfig()

	w.WriteHeader(http.StatusNoContent)
}

// ---------- Engine ----------

type engineStatusResponse struct {
	Running   bool   `json:"running"`
	FlowCount int    `json:"flow_count"`
	Uptime    string `json:"uptime"`
}

func (s *Server) handleEngineStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, engineStatusResponse{
		Running:   s.engine.Running(),
		FlowCount: s.engine.FlowCount(),
		Uptime:    time.Since(s.startTime).Truncate(time.Second).String(),
	})
}

func (s *Server) handleEngineStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.engine.Start()
	writeJSON(w, http.StatusOK, map[string]string{"status": "started"})
}

func (s *Server) handleEngineStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.engine.Stop()
	writeJSON(w, http.StatusOK, map[string]string{"status": "stopped"})
}

// ---------- Segments ----------

type segmentResponse struct {
	CIDR     string            `json:"cidr"`
	Machines []machineResponse `json:"machines"`
}

func (s *Server) handleSegments(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	segments := s.config.BuildSegments()
	resp := make([]segmentResponse, 0, len(segments))
	for _, seg := range segments {
		cidr := seg.CIDR.String()
		// Label non-RFC1918 group as "PUBLIC"
		if cidr == "0.0.0.0/0" {
			cidr = "PUBLIC"
		}
		sr := segmentResponse{
			CIDR:     cidr,
			Machines: make([]machineResponse, 0, len(seg.Machines)),
		}
		for _, m := range seg.Machines {
			ones, _ := m.Mask.Size()
			if cidr == "PUBLIC" {
				ones = 0
			}
			sr.Machines = append(sr.Machines, machineResponse{
				Name: m.Name,
				IP:   m.IP.String(),
				Mask: ones,
			})
		}
		resp = append(resp, sr)
	}
	writeJSON(w, http.StatusOK, resp)
}

// ---------- Configs ----------

func (s *Server) handleConfigs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.listConfigs(w, r)
}

func (s *Server) handleConfigAction(w http.ResponseWriter, r *http.Request) {
	action := strings.TrimPrefix(r.URL.Path, "/api/configs/")
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	switch action {
	case "save":
		s.saveCurrentConfig(w, r)
	case "save-as":
		s.saveConfigAs(w, r)
	case "open":
		s.openConfig(w, r)
	case "new":
		s.newConfig(w, r)
	default:
		http.Error(w, "unknown action", http.StatusNotFound)
	}
}

type configListResponse struct {
	Configs []string `json:"configs"`
	Current string   `json:"current"`
}

func (s *Server) listConfigs(w http.ResponseWriter, _ *http.Request) {
	dir := filepath.Dir(s.configPath)
	entries, err := os.ReadDir(dir)
	if err != nil {
		http.Error(w, "cannot read configs directory: "+err.Error(), http.StatusInternalServerError)
		return
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && (strings.HasSuffix(e.Name(), ".yaml") || strings.HasSuffix(e.Name(), ".yml")) {
			names = append(names, e.Name())
		}
	}
	current := filepath.Base(s.configPath)
	writeJSON(w, http.StatusOK, configListResponse{Configs: names, Current: current})
}

func (s *Server) saveCurrentConfig(w http.ResponseWriter, _ *http.Request) {
	if err := s.saveConfig(); err != nil {
		http.Error(w, "save failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "saved", "file": filepath.Base(s.configPath)})
}

func (s *Server) saveConfigAs(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if err := readJSON(r, &req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.Name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}
	// Sanitize: strip path separators, ensure .yaml extension
	name := filepath.Base(req.Name)
	if !strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml") {
		name = name + ".yaml"
	}
	dir := filepath.Dir(s.configPath)
	newPath := filepath.Join(dir, name)
	if err := config.SaveConfig(s.config, newPath); err != nil {
		http.Error(w, "save failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	s.configPath = newPath
	writeJSON(w, http.StatusOK, map[string]string{"status": "saved", "file": name})
}

func (s *Server) openConfig(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if err := readJSON(r, &req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.Name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}

	dir := filepath.Dir(s.configPath)
	name := filepath.Base(req.Name)
	if !strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml") {
		name = name + ".yaml"
	}
	newPath := filepath.Join(dir, name)

	newCfg, err := config.LoadConfig(newPath)
	if err != nil {
		if os.IsNotExist(err) {
			http.Error(w, "config not found: "+name, http.StatusNotFound)
		} else {
			http.Error(w, "load failed: "+err.Error(), http.StatusInternalServerError)
		}
		return
	}

	// Stop engine before swapping config
	s.engine.Stop()

	// Remove all existing flows and machines from engine
	for _, f := range s.engine.Flows() {
		_ = s.engine.RemoveFlow(f.Name)
	}
	for _, m := range s.engine.Machines() {
		s.engine.RemoveMachine(m.Name)
	}

	// Load new machines
	for _, mc := range newCfg.Machines {
		m, err := mc.ToMachine()
		if err != nil {
			continue
		}
		s.engine.AddMachine(m)
	}

	// Load new flows
	for _, fc := range newCfg.Flows {
		f, err := fc.ToFlow()
		if err != nil {
			continue
		}
		_ = s.engine.AddFlow(f)
	}

	// Swap config and path
	*s.config = *newCfg
	s.configPath = newPath

	writeJSON(w, http.StatusOK, map[string]string{"status": "loaded", "file": filepath.Base(newPath)})
}

func (s *Server) newConfig(w http.ResponseWriter, _ *http.Request) {
	// Stop engine
	s.engine.Stop()

	// Remove all existing flows and machines from engine
	for _, f := range s.engine.Flows() {
		_ = s.engine.RemoveFlow(f.Name)
	}
	for _, m := range s.engine.Machines() {
		s.engine.RemoveMachine(m.Name)
	}

	// Replace config with empty
	s.config.Machines = nil
	s.config.Flows = nil
	s.config.Collectors = nil
	s.configPath = filepath.Join(filepath.Dir(s.configPath), "untitled.yaml")

	writeJSON(w, http.StatusOK, map[string]string{"status": "new", "file": "untitled.yaml"})
}

// ---------- Anomalies ----------

func (s *Server) handleAnomalyScenarios(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	scenarios := anomaly.AllScenarios()
	type scenarioResp struct {
		Type        string  `json:"type"`
		Name        string  `json:"name"`
		Description string  `json:"description"`
		Duration    string  `json:"default_duration"`
		Intensity   float64 `json:"default_intensity"`
		Count       int     `json:"default_count"`
	}
	resp := make([]scenarioResp, len(scenarios))
	for i, sc := range scenarios {
		resp[i] = scenarioResp{
			Type:        string(sc.Type),
			Name:        sc.Name,
			Description: sc.Description,
			Duration:    sc.DefaultDuration.String(),
			Intensity:   sc.DefaultIntensity,
			Count:       sc.DefaultCount,
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleAnomalyActive(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	active := s.engine.ActiveAnomalies()
	type anomalyResp struct {
		ID        string  `json:"id"`
		Scenario  string  `json:"scenario"`
		Name      string  `json:"name"`
		Duration  string  `json:"duration"`
		Intensity float64 `json:"intensity"`
		Remaining string  `json:"remaining"`
	}
	resp := make([]anomalyResp, len(active))
	for i, a := range active {
		resp[i] = anomalyResp{
			ID:        a.ID,
			Scenario:  string(a.Scenario.Type),
			Name:      a.Scenario.Name,
			Duration:  a.Duration.String(),
			Intensity: a.Intensity,
			Remaining: a.Remaining().Truncate(time.Second).String(),
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleAnomalyStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Scenario  string   `json:"scenario"`
		Duration  string   `json:"duration"`
		Intensity float64  `json:"intensity"`
		Targets   []string `json:"targets"`
		Count     int      `json:"count"`
	}
	if err := readJSON(r, &req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	var scenario anomaly.Scenario
	found := false
	for _, sc := range anomaly.AllScenarios() {
		if string(sc.Type) == req.Scenario {
			scenario = sc
			found = true
			break
		}
	}
	if !found {
		http.Error(w, "unknown scenario: "+req.Scenario, http.StatusBadRequest)
		return
	}

	dur := scenario.DefaultDuration
	if req.Duration != "" {
		if d, err := time.ParseDuration(req.Duration); err == nil {
			dur = d
		}
	}
	intensity := scenario.DefaultIntensity
	if req.Intensity > 0 {
		intensity = req.Intensity
	}
	count := scenario.DefaultCount
	if req.Count > 0 {
		count = req.Count
	}

	id, err := s.engine.StartAnomaly(scenario, dur, intensity, req.Targets, count)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Broadcast to WebSocket clients
	data, _ := json.Marshal(map[string]any{
		"type": "anomaly_started",
		"data": map[string]any{
			"id": id, "scenario": req.Scenario, "name": scenario.Name,
			"duration": dur.String(), "targets": req.Targets,
		},
	})
	s.broadcast(data)

	writeJSON(w, http.StatusOK, map[string]string{"id": id, "status": "started"})
}

func (s *Server) handleAnomalyStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		ID string `json:"id"`
	}
	if err := readJSON(r, &req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.engine.StopAnomaly(req.ID); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	data, _ := json.Marshal(map[string]any{
		"type": "anomaly_ended",
		"data": map[string]string{"id": req.ID},
	})
	s.broadcast(data)
	writeJSON(w, http.StatusOK, map[string]string{"status": "stopped"})
}

func (s *Server) handleAnomalyClear(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.engine.ClearAnomalies()
	data, _ := json.Marshal(map[string]any{
		"type": "anomaly_cleared",
		"data": map[string]any{},
	})
	s.broadcast(data)
	writeJSON(w, http.StatusOK, map[string]string{"status": "cleared"})
}

// ---------- WebSocket ----------

func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	hj, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "websocket not supported", http.StatusInternalServerError)
		return
	}

	if !headerContains(r.Header, "Upgrade", "websocket") {
		http.Error(w, "not a websocket request", http.StatusBadRequest)
		return
	}

	wsKey := r.Header.Get("Sec-WebSocket-Key")
	if wsKey == "" {
		http.Error(w, "missing Sec-WebSocket-Key", http.StatusBadRequest)
		return
	}

	acceptKey := computeAcceptKey(wsKey)

	conn, bufrw, err := hj.Hijack()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	bufrw.WriteString("HTTP/1.1 101 Switching Protocols\r\n")
	bufrw.WriteString("Upgrade: websocket\r\n")
	bufrw.WriteString("Connection: Upgrade\r\n")
	bufrw.WriteString("Sec-WebSocket-Accept: " + acceptKey + "\r\n")
	bufrw.WriteString("\r\n")
	bufrw.Flush()

	client := &wsClient{
		send: make(chan []byte, 64),
		done: make(chan struct{}),
	}

	s.wsMu.Lock()
	s.wsClients[client] = struct{}{}
	s.wsMu.Unlock()

	// Writer goroutine: sends messages to the client.
	go func() {
		defer func() {
			s.wsMu.Lock()
			delete(s.wsClients, client)
			s.wsMu.Unlock()
			conn.Close()
		}()
		for {
			select {
			case msg, ok := <-client.send:
				if !ok {
					return
				}
				if err := writeWSFrame(conn, msg); err != nil {
					return
				}
			case <-client.done:
				return
			}
		}
	}()

	// Reader goroutine: reads and discards client messages, detects close.
	go func() {
		defer close(client.done)
		buf := make([]byte, 512)
		for {
			_, err := conn.Read(buf)
			if err != nil {
				return
			}
		}
	}()
}

// headerContains checks if any value of a header contains the target (case insensitive).
func headerContains(h http.Header, key, target string) bool {
	for _, v := range h[key] {
		if strings.EqualFold(v, target) {
			return true
		}
	}
	return false
}

// computeAcceptKey computes the Sec-WebSocket-Accept header value per RFC 6455.
func computeAcceptKey(key string) string {
	const wsMagicGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"
	h := sha1.New()
	h.Write([]byte(key + wsMagicGUID))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

// writeWSFrame writes a text frame to a WebSocket connection.
func writeWSFrame(conn net.Conn, payload []byte) error {
	frame := []byte{0x81} // FIN + text opcode
	pLen := len(payload)
	switch {
	case pLen <= 125:
		frame = append(frame, byte(pLen))
	case pLen <= 65535:
		frame = append(frame, 126, byte(pLen>>8), byte(pLen))
	default:
		frame = append(frame, 127,
			byte(pLen>>56), byte(pLen>>48), byte(pLen>>40), byte(pLen>>32),
			byte(pLen>>24), byte(pLen>>16), byte(pLen>>8), byte(pLen))
	}
	frame = append(frame, payload...)
	_, err := conn.Write(frame)
	return err
}
