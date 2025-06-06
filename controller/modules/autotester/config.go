// controller/modules/autotester/config.go
package autotester

// Bucket is the DB bucket for Auto‑Tester configs/results.
const Bucket = "autotester"

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

	// Usage per test (mL)
	ReagentUseCa  float32 `json:"reagent_use_ca"`
	ReagentUseAlk float32 `json:"reagent_use_alk"`
	ReagentUseMg  float32 `json:"reagent_use_mg"`
	ReagentUseNo3 float32 `json:"reagent_use_no3"`
	ReagentUsePo4 float32 `json:"reagent_use_po4"`

	// Starting volumes (mL)
	ReagentStartCa  float32 `json:"reagent_start_ca"`
	ReagentStartAlk float32 `json:"reagent_start_alk"`
	ReagentStartMg  float32 `json:"reagent_start_mg"`
	ReagentStartNo3 float32 `json:"reagent_start_no3"`
	ReagentStartPo4 float32 `json:"reagent_start_po4"`

	// Remaining volumes (mL)
	ReagentRemainCa  float32 `json:"reagent_remain_ca"`
	ReagentRemainAlk float32 `json:"reagent_remain_alk"`
	ReagentRemainMg  float32 `json:"reagent_remain_mg"`
	ReagentRemainNo3 float32 `json:"reagent_remain_no3"`
	ReagentRemainPo4 float32 `json:"reagent_remain_po4"`

	// Waste tracking
	WasteThreshold float32 `json:"waste_threshold"`
	WasteRemaining float32 `json:"waste_remaining"`
}
