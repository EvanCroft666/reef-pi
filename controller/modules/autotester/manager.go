package autotester

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
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
	configBucket  = "autotester_config"
	resultsBucket = "autotester_readings"
	queueBucket   = "autotester_queue"
)

// Config holds the Auto-Tester settings.
type Config struct {
	ID          string `json:"id"`
	Enable      bool   `json:"enable"`
	I2CAddr     byte   `json:"i2c_addr"`
	EnableCa    bool   `json:"enable_ca"`
	EnableAlk   bool   `json:"enable_alk"`
	EnableMg    bool   `json:"enable_mg"`
	EnableNo3   bool   `json:"enable_no3"`
	EnablePo4   bool   `json:"enable_po4"`
	ScheduleCa  string `json:"schedule_ca"`
	ScheduleAlk string `json:"schedule_alk"`
	ScheduleMg  string `json:"schedule_mg"`
	ScheduleNo3 string `json:"schedule_no3"`
	SchedulePo4 string `json:"schedule_po4"`
	// You may add pump_calibration or other fields here.
}

// Controller implements controller.Subsystem.
type Controller struct {
	c        controller.Controller
	bus      i2c.Bus
	devMode  bool
	queue    *Queue
	logs     []string
	mu       sync.Mutex
	quitters map[string]chan struct{}
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

// Setup bootstraps a default config.
func (m *Controller) Setup() error {
	if _, err := m.Get("default"); err != nil {
		defaultCfg := Config{
			ID:      "default",
			I2CAddr: 0x10,
		}
		return m.CreateOrUpdate(defaultCfg)
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
	// 3) Per-parameter schedule loops

	if cfg.EnableCa {
		q := make(chan struct{})
		m.quitters["ca"] = q
		StartSchedule(cfg.ScheduleCa, q, func() {
			m.queue.AddTask("ca", 0x11)
			m.appendLog("CA: Scheduled test enqueued")
		})
	}
	if cfg.EnableAlk {
		q := make(chan struct{})
		m.quitters["alk"] = q
		StartSchedule(cfg.ScheduleAlk, q, func() {
			m.queue.AddTask("alk", 0x12)
			m.appendLog("ALK: Scheduled test enqueued")
		})
	}
	if cfg.EnableMg {
		q := make(chan struct{})
		m.quitters["mg"] = q
		StartSchedule(cfg.ScheduleMg, q, func() {
			m.queue.AddTask("mg", 0x13)
			m.appendLog("MG: Scheduled test enqueued")
		})
	}
	if cfg.EnableNo3 {
		q := make(chan struct{})
		m.quitters["no3"] = q
		StartSchedule(cfg.ScheduleNo3, q, func() {
			m.queue.AddTask("no3", 0x14)
			m.appendLog("NO3: Scheduled test enqueued")
		})
	}
	if cfg.EnablePo4 {
		q := make(chan struct{})
		m.quitters["po4"] = q
		StartSchedule(cfg.SchedulePo4, q, func() {
			m.queue.AddTask("po4", 0x15)
			m.appendLog("PO4: Scheduled test enqueued")
		})
	}
}

// executeTask runs a single queued Task (test or calibration).
func (m *Controller) executeTask(task Task) {
	param := task.Param
	code := task.Code
	label := strings.ToUpper(param)

	m.appendLog(fmt.Sprintf("%s: Test started", label))

	// Dev-mode: simulate immediately
	if m.devMode {
		fake := float32(rand.Float32()*10.0 + 5.0)
		m.storeResult(param, fake)
		m.appendLog(fmt.Sprintf("%s: Test completed (simulated %.2f)", label, fake))
		return
	}

	// 1) Send START
	if err := m.bus.WriteBytes(m.getConfigI2C(), []byte{code}); err != nil {
		m.appendLog(fmt.Sprintf("%s: Start error: %v", label, err))
		return
	}

	// 2) Poll status
	start := time.Now()
	for {
		time.Sleep(500 * time.Millisecond)
		if err := m.bus.WriteBytes(m.getConfigI2C(), []byte{0x31}); err != nil {
			m.appendLog(fmt.Sprintf("%s: Status write err: %v", label, err))
			return
		}
		st, err := m.bus.ReadBytes(m.getConfigI2C(), 1)
		if err != nil {
			m.appendLog(fmt.Sprintf("%s: Status read err: %v", label, err))
			return
		}
		if st[0] == 2 { // error
			m.appendLog(fmt.Sprintf("%s: Device reported error", label))
			return
		}
		if st[0] == 0 { // idle => done
			break
		}
		if time.Since(start) > 5*time.Minute {
			m.appendLog(fmt.Sprintf("%s: Test timed out", label))
			return
		}
	}

	// 3) READ_RESULT
	if err := m.bus.WriteBytes(m.getConfigI2C(), []byte{0x32}); err != nil {
		m.appendLog(fmt.Sprintf("%s: Result write err: %v", label, err))
		return
	}
	data, err := m.bus.ReadBytes(m.getConfigI2C(), 4)
	if err != nil {
		m.appendLog(fmt.Sprintf("%s: Result read err: %v", label, err))
		return
	}
	value := math.Float32frombits(binary.LittleEndian.Uint32(data))

	// 4) Store and log
	m.storeResult(param, value)
	m.appendLog(fmt.Sprintf("%s: Test completed (%.2f)", label, value))
}

// storeResult persists a float32 reading.
func (m *Controller) storeResult(param string, value float32) {
	rec := struct {
		ID    string  `json:"id"`
		Param string  `json:"param"`
		Time  int64   `json:"ts"`
		Value float32 `json:"value"`
	}{Param: param, Time: time.Now().Unix(), Value: value}

	fn := func(id string) interface{} {
		rec.ID = id
		return &rec
	}
	if err := m.c.Store().Create(resultsBucket, fn); err != nil {
		m.appendLog(fmt.Sprintf("%s: Store error: %v", strings.ToUpper(param), err))
	}
}

// appendLog adds a timestamped message to the in-memory log.
func (m *Controller) appendLog(msg string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	line := fmt.Sprintf("[%s] %s", time.Now().Format("15:04:05"), msg)
	m.logs = append(m.logs, line)
	if len(m.logs) > 100 {
		m.logs = m.logs[1:]
	}
}

// LoadAPI registers all REST endpoints.
func (m *Controller) LoadAPI(r *mux.Router) {
	sr := r.PathPrefix("/api/autotester").Subrouter()
	sr.HandleFunc("/config", m.getConfig).Methods("GET")
	sr.HandleFunc("/config", m.putConfig).Methods("PUT")
	sr.HandleFunc("/run/{param}", m.runOne).Methods("POST")
	sr.HandleFunc("/calibrate/{param}", m.calibrateOne).Methods("POST")
	sr.HandleFunc("/status/{param}", m.statusOne).Methods("GET")
	sr.HandleFunc("/results/{param}", m.resultsOne).Methods("GET")
	// New queue/log endpoints:
	sr.HandleFunc("/queue", m.queueList).Methods("GET")
	sr.HandleFunc("/queue/{param}", m.queueCancel).Methods("DELETE")
	sr.HandleFunc("/log", m.logList).Methods("GET")
}

func (m *Controller) getConfig(w http.ResponseWriter, r *http.Request) {
	cfg, err := m.Get("default")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(cfg)
}

func (m *Controller) putConfig(w http.ResponseWriter, r *http.Request) {
	var cfg Config
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	cfg.ID = "default"
	if err := m.CreateOrUpdate(cfg); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (m *Controller) runOne(w http.ResponseWriter, r *http.Request) {
	key := mux.Vars(r)["param"]
	codeMap := map[string]byte{"ca": 0x11, "alk": 0x12, "mg": 0x13, "no3": 0x14, "po4": 0x15}
	code, ok := codeMap[key]
	if !ok {
		http.Error(w, "Unknown parameter", http.StatusBadRequest)
		return
	}
	if err := m.queue.AddTask(key, code); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	m.appendLog(fmt.Sprintf("%s: Manual test enqueued", strings.ToUpper(key)))
	w.WriteHeader(http.StatusNoContent)
}

func (m *Controller) calibrateOne(w http.ResponseWriter, r *http.Request) {
	key := mux.Vars(r)["param"]
	codeMap := map[string]byte{"pump": 0x21, "ca": 0x22, "alk": 0x23, "mg": 0x24, "no3": 0x25, "po4": 0x26}
	code, ok := codeMap[key]
	if !ok {
		http.Error(w, "Unknown calibration param", http.StatusBadRequest)
		return
	}
	if err := m.queue.AddTask(key, code); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	m.appendLog(fmt.Sprintf("%s: Calibration enqueued", strings.ToUpper(key)))
	w.WriteHeader(http.StatusNoContent)
}

func (m *Controller) statusOne(w http.ResponseWriter, r *http.Request) {
	if m.devMode {
		json.NewEncoder(w).Encode(map[string]int{"status": 0})
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
	json.NewEncoder(w).Encode(map[string]int{"status": int(st[0])})
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

// getConfigI2C returns the saved I²C addr, defaulting to 0x10.
func (m *Controller) getConfigI2C() byte {
	cfg, err := m.Get("default")
	if err != nil {
		return 0x10
	}
	return cfg.I2CAddr
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
	fn := func(id string) interface{} {
		cfg.ID = id
		return &cfg
	}
	if err := m.c.Store().Create(configBucket, fn); err != nil {
		return m.c.Store().Update(configBucket, cfg.ID, &cfg)
	}
	return nil
}

// Stub methods to satisfy controller.Subsystem

func (m *Controller) InUse(string, string) ([]string, error)      { return nil, nil }
func (m *Controller) On(string, bool) error                       { return nil }
func (m *Controller) Stop()                                       {}
func (m *Controller) GetEntity(string) (controller.Entity, error) { return nil, nil }
