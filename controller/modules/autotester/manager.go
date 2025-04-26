// controller/modules/autotester/manager.go
package autotester

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/reef-pi/reef-pi/controller"
	"github.com/reef-pi/rpi/i2c"
)

// Config holds one POC instance.  Here we only ever use ID=="led".
type Config struct {
	ID     string `json:"id"`
	Enable bool   `json:"enable"`
	Addr   byte   `json:"i2c_addr"` // e.g. 0x10
}

// Controller implements controller.Subsystem.
type Controller struct {
	c        controller.Controller
	bus      i2c.Bus
	devMode  bool
	quitters map[string]chan struct{}
}

// New constructs the Auto‑Tester POC subsystem.
func New(devMode bool, c controller.Controller) (*Controller, error) {
	bus, err := i2c.New() // opens /dev/i2c-1
	if err != nil {
		return nil, err
	}
	return &Controller{
		c:        c,
		bus:      bus,
		devMode:  devMode,
		quitters: make(map[string]chan struct{}),
	}, nil
}

// Setup is called once at startup; no POC action here.
func (m *Controller) Setup() error { return nil }

// Start looks for enabled configs and kicks off Run loops.
func (m *Controller) Start() {
	configs, err := m.List()
	if err != nil {
		m.c.LogError("autotester", fmt.Sprintf("failed to list configs: %v", err))
		return
	}
	for _, cfg := range configs {
		if !cfg.Enable {
			continue
		}
		quit := make(chan struct{})
		m.quitters[cfg.ID] = quit
		go m.Run(cfg, quit)
	}
}

// LoadAPI mounts handlers under /api/autotester.
func (m *Controller) LoadAPI(r *mux.Router) {
	sr := r.PathPrefix("/api/autotester").Subrouter()
	sr.HandleFunc("", m.getConfig).Methods("GET")
	sr.HandleFunc("", m.updateConfig).Methods("PUT")
	sr.HandleFunc("/led/on", m.ledOn).Methods("POST")
	sr.HandleFunc("/led/off", m.ledOff).Methods("POST")
}

// The following satisfy controller.Subsystem but do nothing for now:
func (m *Controller) InUse(string, string) ([]string, error)      { return nil, nil }
func (m *Controller) On(string, bool) error                       { return nil }
func (m *Controller) Stop()                                       {}
func (m *Controller) GetEntity(string) (controller.Entity, error) { return nil, nil }

// getConfig / updateConfig serve the single Config (ID="led").
func (m *Controller) getConfig(w http.ResponseWriter, r *http.Request) {
	cfg, err := m.Get("led")
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	json.NewEncoder(w).Encode(cfg)
}

func (m *Controller) updateConfig(w http.ResponseWriter, r *http.Request) {
	var cfg Config
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	cfg.ID = "led"
	if err := m.CreateOrUpdate(cfg); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.WriteHeader(204)
}

// ledOn / ledOff trigger the same POC logic on demand.
func (m *Controller) ledOn(w http.ResponseWriter, r *http.Request) {
	if err := m.poc(0x01); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.WriteHeader(204)
}
func (m *Controller) ledOff(w http.ResponseWriter, r *http.Request) {
	if err := m.poc(0x00); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.WriteHeader(204)
}

// poc writes one byte, reads it back, and logs success or error.
func (m *Controller) poc(cmd byte) error {
	if m.devMode {
		log.Printf("autotester: simulated POC 0x%02x\n", cmd)
		return nil
	}
	addr := byte(0x10) // for now, hard‑coded
	if err := m.bus.WriteBytes(addr, []byte{cmd}); err != nil {
		return err
	}
	resp, err := m.bus.ReadBytes(addr, 1)
	if err != nil {
		return err
	}
	if len(resp) != 1 || resp[0] != cmd {
		return fmt.Errorf("response mismatch: got %x", resp)
	}
	log.Printf("autotester: POC 0x%02x OK\n", cmd)
	return nil
}

// --- CRUD: store & retrieve Config from BoltDB ---

func (m *Controller) Get(id string) (Config, error) {
	var cfg Config
	return cfg, m.c.Store().Get(Bucket, id, &cfg)
}

func (m *Controller) List() ([]Config, error) {
	var list []Config
	fn := func(_ string, v []byte) error {
		var c Config
		_ = json.Unmarshal(v, &c)
		list = append(list, c)
		return nil
	}
	return list, m.c.Store().List(Bucket, fn)
}

func (m *Controller) CreateOrUpdate(cfg Config) error {
	fn := func(id string) interface{} {
		cfg.ID = id
		return &cfg
	}
	if err := m.c.Store().Create(Bucket, fn); err != nil {
		return m.c.Store().Update(Bucket, cfg.ID, cfg)
	}
	return nil
}

// Run is the background loop – here we do the POC every 60s.
func (m *Controller) Run(cfg Config, quit chan struct{}) {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if err := m.poc(0x01); err != nil {
				m.c.LogError("autotester", err.Error())
			}
			if err := m.poc(0x00); err != nil {
				m.c.LogError("autotester", err.Error())
			}
		case <-quit:
			return
		}
	}
}
