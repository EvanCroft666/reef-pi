package autotester

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/reef-pi/reef-pi/controller"
	"github.com/reef-pi/rpi/i2c"
)

// BoltDB buckets
const (
	configBucket  = Bucket // unified bucket name
	resultsBucket = "autotester_readings"
	queueBucket   = "autotester_queue"
)

// Opcodes for reading calibration factors
const (
	OPCODE_READ_PUMP_CALIB = 0x35
	OPCODE_READ_CA_CALIB   = 0x36
	OPCODE_READ_ALK_CALIB  = 0x37
	OPCODE_READ_MG_CALIB   = 0x38
	OPCODE_READ_NO3_CALIB  = 0x39
	OPCODE_READ_PO4_CALIB  = 0x3A
)

// Config is defined in config.go, including all calibration and reagent fields.

// Controller implements controller.Subsystem.
type Controller struct {
	c            controller.Controller
	bus          i2c.Bus
	devMode      bool
	queue        *Queue
	logs         []string
	mu           sync.Mutex
	currentParam string
	quitters     map[string]chan struct{}
}

// New constructs the subsystem and ensures buckets + queue exist.
func New(devMode bool, c controller.Controller) (*Controller, error) {
	// Create BoltDB buckets
	if err := c.Store().CreateBucket(configBucket); err != nil {
		return nil, err
	}
	if err := c.Store().CreateBucket(resultsBucket); err != nil {
		return nil, err
	}
	if err := c.Store().CreateBucket(queueBucket); err != nil {
		return nil, err
	}
	// I2C bus
	bus, err := i2c.New()
	if err != nil {
		return nil, err
	}
	// Persistent queue
	q, err := NewQueue(c.Store())
	if err != nil {
		return nil, err
	}
	return &Controller{
		c:        c,
		bus:      bus,
		devMode:  devMode,
		queue:    q,
		quitters: make(map[string]chan struct{}),
	}, nil
}

// readFloat reads a float32 value from the Arduino
func (m *Controller) readFloat(opcode byte) (float32, error) {
	addr := m.getConfigI2C()
	if err := m.bus.WriteBytes(addr, []byte{opcode}); err != nil {
		return 0, err
	}
	data, err := m.bus.ReadBytes(addr, 4)
	if err != nil {
		return 0, err
	}
	return math.Float32frombits(binary.LittleEndian.Uint32(data)), nil
}

// getConfigI2C returns the I2C address from config
func (m *Controller) getConfigI2C() byte {
	cfg, err := m.Get("default")
	if err != nil {
		return 0x10
	}
	return cfg.I2CAddr
}

// Setup bootstraps a default config.
func (m *Controller) Setup() error {
	// Bootstrap a default config with explicit 'default' ID
	defaultCfg := Config{
		ID:           "default",
		I2CAddr:      0x10,
		ReagentUseCa: 2.0, ReagentUseAlk: 2.0, ReagentUseMg: 2.0, ReagentUseNo3: 2.0, ReagentUsePo4: 2.0,
		ReagentStartCa: 500.0, ReagentStartAlk: 500.0, ReagentStartMg: 500.0, ReagentStartNo3: 500.0, ReagentStartPo4: 500.0,
		ReagentRemainCa: 500.0, ReagentRemainAlk: 500.0, ReagentRemainMg: 500.0, ReagentRemainNo3: 500.0, ReagentRemainPo4: 500.0,
		WasteThreshold: 1000.0,
		WasteRemaining: 0.0,
	}
	// Try updating existing record
	if err := m.c.Store().Update(configBucket, defaultCfg.ID, &defaultCfg); err != nil {
		// If update fails, create new record
		fn := func(id string) interface{} {
			defaultCfg.ID = id
			return &defaultCfg
		}
		if err2 := m.c.Store().Create(configBucket, fn); err2 != nil {
			return err2
		}
	}
	return nil
}

// Start launches the queue worker and per-parameter schedulers.
func (m *Controller) Start() {
	// 1) Queue worker
	go m.queue.ProcessTasks(func(t Task) { m.executeTask(t) })

	// 2) Load config
	cfg, err := m.Get("default")
	if err != nil {
		m.c.LogError("autotester", "load config: "+err.Error())
		return
	}

	// 4) Per-parameter schedule loops
	if cfg.EnableCa {
		q := make(chan struct{})
		m.quitters["ca"] = q
		StartSchedule(cfg.ScheduleCa, q, func() {
			if err := m.canEnqueueTest("ca", cfg.ReagentUseCa); err != nil {
				m.appendLog(fmt.Sprintf("CA: Skipped schedule (%v)", err))
				return
			}
			if err := m.queue.AddTask("ca", 0x11); err != nil {
				m.appendLog(fmt.Sprintf("CA: Skipped schedule (%v)", err))
			} else {
				m.appendLog("CA: Scheduled test enqueued")
			}
		})
	}
	if cfg.EnableAlk {
		q := make(chan struct{})
		m.quitters["alk"] = q
		StartSchedule(cfg.ScheduleAlk, q, func() {
			if err := m.canEnqueueTest("alk", cfg.ReagentUseAlk); err != nil {
				m.appendLog(fmt.Sprintf("ALK: Skipped schedule (%v)", err))
				return
			}
			if err := m.queue.AddTask("alk", 0x12); err != nil {
				m.appendLog(fmt.Sprintf("ALK: Skipped schedule (%v)", err))
			} else {
				m.appendLog("ALK: Scheduled test enqueued")
			}
		})
	}
	if cfg.EnableMg {
		q := make(chan struct{})
		m.quitters["mg"] = q
		StartSchedule(cfg.ScheduleMg, q, func() {
			if err := m.canEnqueueTest("mg", cfg.ReagentUseMg); err != nil {
				m.appendLog(fmt.Sprintf("MG: Skipped schedule (%v)", err))
				return
			}
			if err := m.queue.AddTask("mg", 0x13); err != nil {
				m.appendLog(fmt.Sprintf("MG: Skipped schedule (%v)", err))
			} else {
				m.appendLog("MG: Scheduled test enqueued")
			}
		})
	}
	if cfg.EnableNo3 {
		q := make(chan struct{})
		m.quitters["no3"] = q
		StartSchedule(cfg.ScheduleNo3, q, func() {
			if err := m.canEnqueueTest("no3", cfg.ReagentUseNo3); err != nil {
				m.appendLog(fmt.Sprintf("NO3: Skipped schedule (%v)", err))
				return
			}
			if err := m.queue.AddTask("no3", 0x14); err != nil {
				m.appendLog(fmt.Sprintf("NO3: Skipped schedule (%v)", err))
			} else {
				m.appendLog("NO3: Scheduled test enqueued")
			}
		})
	}
	if cfg.EnablePo4 {
		q := make(chan struct{})
		m.quitters["po4"] = q
		StartSchedule(cfg.SchedulePo4, q, func() {
			if err := m.canEnqueueTest("po4", cfg.ReagentUsePo4); err != nil {
				m.appendLog(fmt.Sprintf("PO4: Skipped schedule (%v)", err))
				return
			}
			if err := m.queue.AddTask("po4", 0x15); err != nil {
				m.appendLog(fmt.Sprintf("PO4: Skipped schedule (%v)", err))
			} else {
				m.appendLog("PO4: Scheduled test enqueued")
			}
		})
	}
}

// canEnqueueTest ensures reagent and waste are within safe limits.
func (m *Controller) canEnqueueTest(param string, use float32) error {
	cfg, err := m.Get("default")
	if err != nil {
		return err
	}
	switch param {
	case "ca":
		if cfg.ReagentRemainCa < use {
			return fmt.Errorf("CA reagent empty")
		}
	case "alk":
		if cfg.ReagentRemainAlk < use {
			return fmt.Errorf("ALK reagent empty")
		}
	case "mg":
		if cfg.ReagentRemainMg < use {
			return fmt.Errorf("MG reagent empty")
		}
	case "no3":
		if cfg.ReagentRemainNo3 < use {
			return fmt.Errorf("NO3 reagent empty")
		}
	case "po4":
		if cfg.ReagentRemainPo4 < use {
			return fmt.Errorf("PO4 reagent empty")
		}
	default:
		return nil
	}
	if cfg.WasteRemaining+use > cfg.WasteThreshold {
		return fmt.Errorf("waste threshold reached")
	}
	return nil
}

// executeTask runs a single queued Task (test or flush).
func (m *Controller) executeTask(task Task) {
	param := task.Param
	code := task.Code

	// Mark busy parameter (include flush_ prefix)
	m.mu.Lock()
	m.currentParam = param
	m.mu.Unlock()
	// Human‐friendly label (strip flush_ prefix)
	label := strings.ToUpper(strings.TrimPrefix(param, "flush_"))
	// Log initial event based on task type
	if strings.HasPrefix(param, "flush_") {
		m.appendLog(fmt.Sprintf("%s: Flush started", label))
	} else if param == "pump" {
		// pump calibration logs handled in its branch
	} else if strings.HasPrefix(param, "cal_") {
		real := strings.TrimPrefix(param, "cal_")
		label := strings.ToUpper(real)
		// Send START command
		if err := m.bus.WriteBytes(m.getConfigI2C(), []byte{code}); err != nil {
			m.appendLog(fmt.Sprintf("%s: Calibration start error: %v", label, err))
			return
		}
		// Poll until idle
		for {
			time.Sleep(500 * time.Millisecond)
			_ = m.bus.WriteBytes(m.getConfigI2C(), []byte{0x31})
			st, err := m.bus.ReadBytes(m.getConfigI2C(), 1)
			if err != nil {
				m.appendLog(fmt.Sprintf("%s: Status error: %v", label, err))
				return
			}
			if st[0] == 0 {
				break
			}
		}
		// Give Arduino time to update calibration factor
		time.Sleep(100 * time.Millisecond)
		// Read and log final factor
		var readOp byte
		switch real {
		case "ca":
			readOp = OPCODE_READ_CA_CALIB
		case "alk":
			readOp = OPCODE_READ_ALK_CALIB
		case "mg":
			readOp = OPCODE_READ_MG_CALIB
		case "no3":
			readOp = OPCODE_READ_NO3_CALIB
		case "po4":
			readOp = OPCODE_READ_PO4_CALIB
		default:
			readOp = code
		}
		val, err := m.readFloat(readOp)
		if err == nil {
			m.appendLog(fmt.Sprintf("%s: Calibration finished. Updated calibration factor: %.3f", label, val))
		}
		// Clear busy flag
		m.mu.Lock()
		m.currentParam = ""
		m.mu.Unlock()
		return
	} else {
		m.appendLog(fmt.Sprintf("%s: Test started", label))
	}

	// ── Handle flush tasks separately ──
	if strings.HasPrefix(param, "flush_") {
		real := strings.TrimPrefix(param, "flush_")
		label = strings.ToUpper(real)
		m.appendLog(fmt.Sprintf("%s: Flush started", label))

		// 1) Send START (flush)
		if err := m.bus.WriteBytes(m.getConfigI2C(), []byte{code}); err != nil {
			m.appendLog(fmt.Sprintf("%s: Flush start error: %v", label, err))
			return
		}
		// 2) Poll status until idle or timeout
		for {
			time.Sleep(500 * time.Millisecond)
			_ = m.bus.WriteBytes(m.getConfigI2C(), []byte{0x31})
			st, err := m.bus.ReadBytes(m.getConfigI2C(), 1)
			if err != nil {
				m.appendLog(fmt.Sprintf("%s: Flush status err: %v", label, err))
				return
			}
			if st[0] == 0 {
				break
			}
		}
		m.appendLog(fmt.Sprintf("%s: Flush completed", label))

		// Volume accounting for flush — only reset when flush succeeds
		if cfg, err := m.Get("default"); err == nil {
			switch real {
			case "ca":
				cfg.ReagentRemainCa = cfg.ReagentStartCa
			case "alk":
				cfg.ReagentRemainAlk = cfg.ReagentStartAlk
			case "mg":
				cfg.ReagentRemainMg = cfg.ReagentStartMg
			case "no3":
				cfg.ReagentRemainNo3 = cfg.ReagentStartNo3
			case "po4":
				cfg.ReagentRemainPo4 = cfg.ReagentStartPo4
			}
			_ = m.CreateOrUpdate(cfg)
			m.appendLog(fmt.Sprintf("%s: Reagent refilled to start volume; Waste remaining: %.1f mL", label, cfg.WasteRemaining))
		}
		return
	}

	// ── Pump calibration ──
	if param == "pump" {
		m.appendLog("PUMP: Calibration test started. Pumping 50.0 mL. Please wait...")
		// Send START command for pump calibration
		if err := m.bus.WriteBytes(m.getConfigI2C(), []byte{code}); err != nil {
			m.appendLog(fmt.Sprintf("PUMP: Start error: %v", err))
			return
		}
		// Poll status until idle (no timeout)
		for {
			time.Sleep(500 * time.Millisecond)
			_ = m.bus.WriteBytes(m.getConfigI2C(), []byte{0x31})
			st, err := m.bus.ReadBytes(m.getConfigI2C(), 1)
			if err != nil {
				m.appendLog(fmt.Sprintf("PUMP: Status error: %v", err))
				return
			}
			if st[0] == 0 {
				break
			}
		}
		m.appendLog("PUMP: Pumping finished. Please enter dispensed volume")
		// Clear busy flag so status returns to idle
		m.mu.Lock()
		m.currentParam = ""
		m.mu.Unlock()
		return
	} else {
		// ── Normal test sequence ──

		// 1) Send START command
		if err := m.bus.WriteBytes(m.getConfigI2C(), []byte{code}); err != nil {
			m.appendLog(fmt.Sprintf("%s: Start error: %v", label, err))
			return
		}

		// 2) Poll status until idle (0), error (2), or timeout
		for {
			time.Sleep(500 * time.Millisecond)
			_ = m.bus.WriteBytes(m.getConfigI2C(), []byte{0x31})
			st, err := m.bus.ReadBytes(m.getConfigI2C(), 1)
			if err != nil {
				m.appendLog(fmt.Sprintf("%s: Status err: %v", label, err))
				return
			}
			if st[0] == 2 {
				m.appendLog(fmt.Sprintf("%s: Device reported error", label))
				return
			}
			if st[0] == 0 {
				break
			}
		}

		// 3) READ_RESULT
		_ = m.bus.WriteBytes(m.getConfigI2C(), []byte{0x32})
		m.appendLog(fmt.Sprintf("%s: Sent READ_RESULT command (0x32)", label))
		data, err := m.bus.ReadBytes(m.getConfigI2C(), 4)
		if err != nil {
			m.appendLog(fmt.Sprintf("%s: Result read err: %v", label, err))
			return
		}
		m.appendLog(fmt.Sprintf("%s: Read raw bytes: [%x %x %x %x]", label, data[0], data[1], data[2], data[3]))
		bits := binary.LittleEndian.Uint32(data)
		m.appendLog(fmt.Sprintf("%s: Raw bits: 0x%08x", label, bits))
		value := math.Float32frombits(bits)
		m.appendLog(fmt.Sprintf("%s: Converted to float32: %f", label, value))

		// 4) Store and log
		m.storeResult(param, value)
		m.appendLog(fmt.Sprintf("%s: Test completed (%.2f)", label, value))

		// 5) Update reagent & waste
		if cfg, err := m.Get("default"); err == nil {
			var use, rem float32
			switch param {
			case "ca":
				use = cfg.ReagentUseCa
				cfg.ReagentRemainCa -= use
				rem = cfg.ReagentRemainCa
			case "alk":
				use = cfg.ReagentUseAlk
				cfg.ReagentRemainAlk -= use
				rem = cfg.ReagentRemainAlk
			case "mg":
				use = cfg.ReagentUseMg
				cfg.ReagentRemainMg -= use
				rem = cfg.ReagentRemainMg
			case "no3":
				use = cfg.ReagentUseNo3
				cfg.ReagentRemainNo3 -= use
				rem = cfg.ReagentRemainNo3
			case "po4":
				use = cfg.ReagentUsePo4
				cfg.ReagentRemainPo4 -= use
				rem = cfg.ReagentRemainPo4
			}
			cfg.WasteRemaining += use
			_ = m.CreateOrUpdate(cfg)
			m.appendLog(fmt.Sprintf("%s: Reagent remaining: %.1f mL; Waste: %.1f mL", label, rem, cfg.WasteRemaining))
		}
	}
}

// fillReagent resets a reagent to its start volume and enqueues a flush.
func (m *Controller) fillReagentOne(w http.ResponseWriter, r *http.Request) {
	key := mux.Vars(r)["param"]
	// ← removed immediate reset; will defer until flush completes
	// enqueue flush routine
	codeMap := map[string]byte{"ca": 0x27, "alk": 0x28, "mg": 0x29, "no3": 0x2A, "po4": 0x2B}
	code, ok := codeMap[key]
	if !ok {
		http.Error(w, "No flush code", http.StatusInternalServerError)
		return
	}
	taskParam := "flush_" + key
	if err := m.queue.AddTask(taskParam, code); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	m.appendLog(fmt.Sprintf("%s: Reagent fill & flush enqueued", strings.ToUpper(key)))
	w.WriteHeader(http.StatusNoContent)
}

// enqueuePumpCalibration enqueues the pump calibration task.
func (m *Controller) enqueuePumpCalibration(w http.ResponseWriter, r *http.Request) {
	// Safety: no other tasks running or queued
	m.queue.mu.Lock()
	busy := m.queue.current != nil
	m.queue.mu.Unlock()
	if busy {
		http.Error(w, "Cannot calibrate pump: another task in progress", http.StatusConflict)
		return
	}
	var queuedCount int
	_ = m.queue.store.List(queueBucket, func(_ string, _ []byte) error { queuedCount++; return nil })
	if queuedCount > 0 {
		http.Error(w, "Cannot calibrate pump: queue not empty", http.StatusConflict)
		return
	}
	if err := m.queue.AddTask("pump", 0x21); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	m.appendLog("PUMP: Calibration test started. Pumping 50.0 mL. Please wait...")
	w.WriteHeader(http.StatusNoContent)
}

// enqueueParamCalibration enqueues the parameter calibration task.
func (m *Controller) enqueueParamCalibration(w http.ResponseWriter, r *http.Request) {
	key := mux.Vars(r)["param"]

	// Get known value from request body
	var payload struct {
		Value float32 `json:"value"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}

	// Map param → opcode
	codeMap := map[string]byte{
		"ca":  0x22,
		"alk": 0x23,
		"mg":  0x24,
		"no3": 0x25,
		"po4": 0x26,
	}
	code, ok := codeMap[key]
	if !ok {
		http.Error(w, "Unknown parameter", http.StatusBadRequest)
		return
	}

	// Safety: no current task
	m.queue.mu.Lock()
	busy := m.queue.current != nil
	m.queue.mu.Unlock()
	if busy {
		http.Error(w, "Cannot calibrate: another task in progress", http.StatusConflict)
		return
	}

	// Safety: queue must be empty
	var queued int
	_ = m.queue.store.List(queueBucket, func(_ string, _ []byte) error { queued++; return nil })
	if queued > 0 {
		http.Error(w, "Cannot calibrate: queue not empty", http.StatusConflict)
		return
	}

	// Send known value to Arduino
	buf := make([]byte, 5)
	buf[0] = code
	binary.LittleEndian.PutUint32(buf[1:], math.Float32bits(payload.Value))
	if err := m.bus.WriteBytes(m.getConfigI2C(), buf); err != nil {
		http.Error(w, fmt.Sprintf("I2C write failed: %v", err), http.StatusInternalServerError)
		return
	}
	// Log calibration start
	label := strings.ToUpper(key)
	m.appendLog(fmt.Sprintf("%s: Calibration test started. Known value: %.3f. Please wait...", label, payload.Value))

	// Enqueue under a distinct param name
	taskParam := "cal_" + key
	if err := m.queue.AddTask(taskParam, code); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// LoadAPI registers all REST endpoints.
func (m *Controller) LoadAPI(r *mux.Router) {
	sr := r.PathPrefix("/api/autotester").Subrouter()
	sr.HandleFunc("/config", m.getConfig).Methods("GET")
	sr.HandleFunc("/config", m.putConfig).Methods("PUT")
	sr.HandleFunc("/run/{param}", m.runOne).Methods("POST")
	// 1) Pump‑calibration start endpoint
	sr.HandleFunc("/calibrate/pump/start", m.enqueuePumpCalibration).Methods("POST")
	// 2) Generic calibration endpoint (receives measured volumes)
	sr.HandleFunc("/calibrate/{param}", m.calibrateOne).Methods("POST")
	sr.HandleFunc("/status/{param}", m.statusOne).Methods("GET")
	sr.HandleFunc("/results/{param}", m.resultsOne).Methods("GET")
	// queue, log, cancel
	sr.HandleFunc("/queue", m.queueList).Methods("GET")
	sr.HandleFunc("/queue/{param}", m.queueCancel).Methods("DELETE")
	sr.HandleFunc("/log", m.logList).Methods("GET")
	// new fill endpoint
	sr.HandleFunc("/fill/{param}", m.fillReagentOne).Methods("POST")
	// new param calibration endpoint
	sr.HandleFunc("/calibrate/{param}/start", m.enqueueParamCalibration).Methods("POST")
}

// appendLog adds an entry to the in-memory activity log, capped at 100 entries.
func (m *Controller) appendLog(msg string) {
	entry := fmt.Sprintf("%s %s", time.Now().Format("15:04:05"), msg)
	m.mu.Lock()
	defer m.mu.Unlock()
	m.logs = append(m.logs, entry)
	if len(m.logs) > 100 {
		m.logs = m.logs[len(m.logs)-100:]
	}
}

// storeResult persists a test result into BoltDB.
func (m *Controller) storeResult(param string, value float32) error {
	type record struct {
		Param string  `json:"param"`
		Ts    int64   `json:"ts"`
		Value float32 `json:"value"`
	}
	rec := record{
		Param: param,
		Ts:    time.Now().Unix(),
		Value: value,
	}
	// Always create a new entry so we never overwrite previous results
	// fn will be called with a brand‑new key; the record itself carries its timestamp.
	return m.c.Store().Create(resultsBucket, func(id string) interface{} {
		// Note: we don't store `id` inside rec; BoltDB key is id, rec.Ts has the timestamp.
		r := rec
		return &r
	})
}

func (m *Controller) getConfig(w http.ResponseWriter, r *http.Request) {
	// Try to load existing config; if missing, bootstrap defaults
	cfg, err := m.Get("default")
	if err != nil {
		if setupErr := m.Setup(); setupErr != nil {
			http.Error(w, setupErr.Error(), http.StatusInternalServerError)
			return
		}
		cfg, err = m.Get("default")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	// Create a response struct that includes both config and calibration factors
	type response struct {
		Config
		PumpCalibration float32 `json:"pump_calibration"`
		CalibrationCa   float32 `json:"calibration_ca"`
		CalibrationAlk  float32 `json:"calibration_alk"`
		CalibrationMg   float32 `json:"calibration_mg"`
		CalibrationNo3  float32 `json:"calibration_no3"`
		CalibrationPo4  float32 `json:"calibration_po4"`
	}

	// Read calibration factors directly from Arduino
	resp := response{Config: cfg}

	// Read pump calibration
	if val, err := m.readFloat(OPCODE_READ_PUMP_CALIB); err == nil {
		resp.PumpCalibration = val
	}

	// Read parameter calibrations
	if val, err := m.readFloat(OPCODE_READ_CA_CALIB); err == nil {
		resp.CalibrationCa = val
	}
	if val, err := m.readFloat(OPCODE_READ_ALK_CALIB); err == nil {
		resp.CalibrationAlk = val
	}
	if val, err := m.readFloat(OPCODE_READ_MG_CALIB); err == nil {
		resp.CalibrationMg = val
	}
	if val, err := m.readFloat(OPCODE_READ_NO3_CALIB); err == nil {
		resp.CalibrationNo3 = val
	}
	if val, err := m.readFloat(OPCODE_READ_PO4_CALIB); err == nil {
		resp.CalibrationPo4 = val
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (m *Controller) putConfig(w http.ResponseWriter, r *http.Request) {
	// 1) Decode new config
	var newCfg Config
	if err := json.NewDecoder(r.Body).Decode(&newCfg); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	newCfg.ID = "default"

	// 2) Load existing config for diff
	oldCfg, err := m.Get("default")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// 3) Persist the changes
	if err := m.CreateOrUpdate(newCfg); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// 4) Apply schedule/enable diffs
	m.applyConfigChanges(oldCfg, newCfg)
	// LOG: record that user saved new configuration
	m.appendLog("AutoTester configuration saved")
	w.WriteHeader(http.StatusNoContent)
}

func (m *Controller) runOne(w http.ResponseWriter, r *http.Request) {
	// Retrieve the parameter from the URL
	key := mux.Vars(r)["param"]
	codeMap := map[string]byte{"ca": 0x11, "alk": 0x12, "mg": 0x13, "no3": 0x14, "po4": 0x15}
	code, ok := codeMap[key]
	if !ok {
		http.Error(w, "Unknown parameter", http.StatusBadRequest)
		return
	}
	// Safety: check reagent/waste limits
	cfg, err := m.Get("default")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var use float32
	switch key {
	case "ca":
		use = cfg.ReagentUseCa
	case "alk":
		use = cfg.ReagentUseAlk
	case "mg":
		use = cfg.ReagentUseMg
	case "no3":
		use = cfg.ReagentUseNo3
	case "po4":
		use = cfg.ReagentUsePo4
	}
	if err := m.canEnqueueTest(key, use); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	// Enqueue the task
	if err := m.queue.AddTask(key, code); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	m.appendLog(fmt.Sprintf("%s: Manual test enqueued", strings.ToUpper(key)))
	w.WriteHeader(http.StatusNoContent)
}

func (m *Controller) calibrateOne(w http.ResponseWriter, r *http.Request) {
	key := mux.Vars(r)["param"]
	// 1) Decode new calibration value
	var payload struct {
		Value float32 `json:"value"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}

	// 2) Map param → opcode
	codeMap := map[string]byte{
		"pump": 0x21,
		"ca":   0x22, "alk": 0x23,
		"mg": 0x24, "no3": 0x25,
		"po4": 0x26,
	}
	code, ok := codeMap[key]
	if !ok {
		http.Error(w, "Unknown calibration param", http.StatusBadRequest)
		return
	}

	// 3) Send [opcode, float32 bytes] in one transaction
	buf := make([]byte, 5)
	buf[0] = code
	binary.LittleEndian.PutUint32(buf[1:], math.Float32bits(payload.Value))
	if err := m.bus.WriteBytes(m.getConfigI2C(), buf); err != nil {
		http.Error(w, fmt.Sprintf("I2C write failed: %v", err), http.StatusInternalServerError)
		return
	}

	// 4) Log the calibration value being sent
	label := strings.ToUpper(key)
	if key != "pump" {
		m.appendLog(fmt.Sprintf("%s: Sending calibration value: %.3f", label, payload.Value))
	}

	// 5) Read back the updated value from Arduino
	readOps := map[string]byte{
		"pump": OPCODE_READ_PUMP_CALIB,
		"ca":   OPCODE_READ_CA_CALIB,
		"alk":  OPCODE_READ_ALK_CALIB,
		"mg":   OPCODE_READ_MG_CALIB,
		"no3":  OPCODE_READ_NO3_CALIB,
		"po4":  OPCODE_READ_PO4_CALIB,
	}
	if op, ok := readOps[key]; ok {
		addr := m.getConfigI2C()
		if err := m.bus.WriteBytes(addr, []byte{op}); err != nil {
			m.appendLog(fmt.Sprintf("%s: Read-back cmd error: %v", label, err))
		} else if data, err := m.bus.ReadBytes(addr, 4); err != nil {
			m.appendLog(fmt.Sprintf("%s: Read-back data error: %v", label, err))
		} else {
			newVal := math.Float32frombits(binary.LittleEndian.Uint32(data))
			if key == "pump" {
				m.appendLog(fmt.Sprintf("PUMP: Pump calibration factor updated: %.3f", newVal))
			} else {
				m.appendLog(fmt.Sprintf("%s: Retrieved updated calibration factor: %.3f", label, newVal))
			}
		}
	}

	w.WriteHeader(http.StatusNoContent)
}

func (m *Controller) statusOne(w http.ResponseWriter, r *http.Request) {
	type resp struct {
		Status int    `json:"status"`
		Param  string `json:"param"`
	}
	if m.devMode {
		json.NewEncoder(w).Encode(resp{Status: 0, Param: ""})
		return
	}
	addr := m.getConfigI2C()
	if err := m.bus.WriteBytes(addr, []byte{0x31}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	st, err := m.bus.ReadBytes(addr, 1)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	m.mu.Lock()
	running := m.currentParam
	m.mu.Unlock()
	json.NewEncoder(w).Encode(resp{Status: int(st[0]), Param: running})
}

func (m *Controller) resultsOne(w http.ResponseWriter, r *http.Request) {
	key := mux.Vars(r)["param"]
	var out []map[string]interface{}
	_ = m.c.Store().List(resultsBucket, func(_ string, v []byte) error {
		var rec struct {
			Param string  `json:"param"`
			Time  int64   `json:"ts"` // raw Unix seconds
			Value float32 `json:"value"`
		}
		if err := json.Unmarshal(v, &rec); err == nil && rec.Param == key {
			// formatted HH:MM:SS label
			label := time.Unix(rec.Time, 0).Local().Format("15:04:05")
			out = append(out, map[string]interface{}{
				"ts":    rec.Time,
				"time":  label,
				"value": rec.Value,
			})
		}
		return nil
	})
	json.NewEncoder(w).Encode(out)
}

func (m *Controller) queueList(w http.ResponseWriter, r *http.Request) {
	tasks, err := m.queue.ListTasks()
	if err != nil {
		http.Error(w, "Failed to list queue", http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(tasks)
}

func (m *Controller) queueCancel(w http.ResponseWriter, r *http.Request) {
	key := mux.Vars(r)["param"]
	if err := m.queue.RemoveTask(key); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	m.appendLog(fmt.Sprintf("%s: Pending task canceled", strings.ToUpper(key)))
	w.WriteHeader(http.StatusNoContent)
}

func (m *Controller) logList(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	defer m.mu.Unlock()
	json.NewEncoder(w).Encode(m.logs)
}

// CRUD for Config

func (m *Controller) Get(id string) (Config, error) {
	var cfg Config
	return cfg, m.c.Store().Get(configBucket, id, &cfg)
}

func (m *Controller) List() ([]Config, error) {
	var list []Config
	_ = m.c.Store().List(configBucket, func(_ string, v []byte) error {
		var c Config
		if err := json.Unmarshal(v, &c); err == nil {
			list = append(list, c)
		}
		return nil
	})
	// option: sort list by ID or other
	return list, nil
}

func (m *Controller) CreateOrUpdate(cfg Config) error {
	// First try to update the existing record
	if err := m.c.Store().Update(configBucket, cfg.ID, &cfg); err == nil {
		return nil
	}
	// If it doesn't exist, create it
	fn := func(id string) interface{} {
		cfg.ID = id
		return &cfg
	}
	return m.c.Store().Create(configBucket, fn)
}

// Stub methods to satisfy controller.Subsystem

func (m *Controller) InUse(string, string) ([]string, error)      { return nil, nil }
func (m *Controller) On(string, bool) error                       { return nil }
func (m *Controller) Stop()                                       {}
func (m *Controller) GetEntity(string) (controller.Entity, error) { return nil, nil }

// applyConfigChanges compares old vs new config and starts/stops schedulers accordingly.
func (m *Controller) applyConfigChanges(oldCfg, newCfg Config) {
	// helper to manage one parameter
	manage := func(key string, oldEn, newEn bool, oldSch, newSch string, use float32, opcode byte) {
		// If it was enabled before, but now disabled or schedule cleared → stop it
		if oldEn && (!newEn || oldSch != newSch) {
			if q, ok := m.quitters[key]; ok {
				close(q)
				delete(m.quitters, key)
			}
		}
		// If it is now enabled with a valid schedule, and either just enabled or schedule changed → start it
		if newEn && newSch != "" && (!oldEn || oldSch != newSch) {
			q := make(chan struct{})
			m.quitters[key] = q
			StartSchedule(newSch, q, func() {
				if err := m.canEnqueueTest(key, use); err != nil {
					m.appendLog(fmt.Sprintf("%s: Skipped schedule (%v)", strings.ToUpper(key), err))
					return
				}
				if err := m.queue.AddTask(key, opcode); err == nil {
					m.appendLog(fmt.Sprintf("%s: Scheduled test enqueued", strings.ToUpper(key)))
				}
			})
		}
	}

	// CA
	manage("ca", oldCfg.EnableCa, newCfg.EnableCa, oldCfg.ScheduleCa, newCfg.ScheduleCa,
		oldCfg.ReagentUseCa, 0x11)
	// Alk
	manage("alk", oldCfg.EnableAlk, newCfg.EnableAlk, oldCfg.ScheduleAlk, newCfg.ScheduleAlk,
		oldCfg.ReagentUseAlk, 0x12)
	// Mg
	manage("mg", oldCfg.EnableMg, newCfg.EnableMg, oldCfg.ScheduleMg, newCfg.ScheduleMg,
		oldCfg.ReagentUseMg, 0x13)
	// NO3
	manage("no3", oldCfg.EnableNo3, newCfg.EnableNo3, oldCfg.ScheduleNo3, newCfg.ScheduleNo3,
		oldCfg.ReagentUseNo3, 0x14)
	// PO4
	manage("po4", oldCfg.EnablePo4, newCfg.EnablePo4, oldCfg.SchedulePo4, newCfg.SchedulePo4,
		oldCfg.ReagentUsePo4, 0x15)
}
