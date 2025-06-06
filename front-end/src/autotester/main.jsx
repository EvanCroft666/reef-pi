import React, { useState, useEffect, useRef } from 'react'
import Chart from './chart'
import ScheduleForm from './ScheduleForm'
import QueueLog from './QueueLog'
import LogOutput from './LogOutput'
import ConfigPanel from './ConfigPanel'
// ↑ Removed CalibrationPanel import

const PARAMS = [
  { key: 'ca',  label: 'Calcium',    unit: 'ppm' },
  { key: 'alk', label: 'Alkalinity', unit: 'dKH' },
  { key: 'mg',  label: 'Magnesium',  unit: 'ppm' },
  { key: 'no3', label: 'Nitrate',    unit: 'ppm' },
  { key: 'po4', label: 'Phosphate',  unit: 'ppm' },
]

const DEFAULT_CFG = {
  id:           'default',
  enable:       false,
  i2c_addr:     0x10,
  enable_ca:    false,
  enable_alk:   false,
  enable_mg:    false,
  enable_no3:   false,
  enable_po4:   false,
  schedule_ca:  '',
  schedule_alk: '',
  schedule_mg:  '',
  schedule_no3: '',
  schedule_po4: '',
}

export default function AutoTester() {
  const [cfg, setCfg] = useState(DEFAULT_CFG)
  // NEW: overallStatus replaces per‐param status
  const [overallStatus, setOverallStatus] = useState('Idle')
  const prevStat = useRef({})
  const [data, setData] = useState({})
  const [queue, setQueue] = useState([])
  const [logs, setLogs] = useState([])
  const [scheduleEditParam, setScheduleEditParam] = useState(null)
  const [pumpCalibrating, setPumpCalibrating] = useState(false)
  // Parameter calibration state
  const [paramCalibrating, setParamCalibrating] = useState(null)    // e.g. 'ca', 'alk', etc.
  const [paramValue, setParamValue] = useState('')                 // user-entered reference
  const [pumpVolume, setPumpVolume] = useState('')
  const [showConfig, setShowConfig] = useState(false)

  // Fetch config on mount
  useEffect(() => {
    fetch('/api/autotester/config', { credentials: 'include' })
      .then(r => r.ok ? r.json() : Promise.reject())
      .then(config => {
        // Reset manual‐run checkboxes on each load
        ['ca','alk','mg','no3','po4'].forEach(k => config[`enable_${k}`] = false)
        setCfg(config)
      })
      .catch(() => setCfg(DEFAULT_CFG))
  }, [])

  // Poll status, results, queue, and logs every 2s
  useEffect(() => {
    const iv = setInterval(() => {
      // 1) Global status: use the status from one parameter (e.g. 'ca') as a proxy
      fetch('/api/autotester/status/ca', { credentials: 'include' })
        .then(r => r.json())
        .then(js => {
          let msg = 'Idle'
          if (js.status === 2) {
            msg = 'ERROR'
          } else if (js.status === 1) {
            const p = js.param
            if (p.startsWith('flush_')) {
              msg = `Flushing: ${p.replace('flush_','').toUpperCase()}`
            } else if (p === 'pump') {
              msg = 'Calibrating Pump'
            } else {
              msg = `Testing: ${p.toUpperCase()}`
            }
          }
          setOverallStatus(msg)
        })
      // 2) Queue list & logs
      fetch('/api/autotester/queue', { credentials: 'include' })
        .then(r => r.json()).then(setQueue)
      fetch('/api/autotester/log', { credentials: 'include' })
        .then(r => r.json()).then(setLogs)
    }, 2000)
    return () => clearInterval(iv)
  }, [])
  
// Poll config every 5 seconds to pick up reagent & waste updates
useEffect(() => {
  const iv2 = setInterval(() => {
    fetch('/api/autotester/config', { credentials: 'include' })
      .then(r => r.ok ? r.json() : Promise.reject())
      .then(setCfg)
      .catch(() => {/* ignore */});
  }, 5000);
  return () => clearInterval(iv2);
}, []);

  // Load historical data on config change
  useEffect(() => {
    PARAMS.forEach(({ key }) => {
      fetch(`/api/autotester/results/${key}`, { credentials: 'include' })
        .then(r => r.json())
        .then(arr => setData(d => ({ ...d, [key]: arr })))
    })
  }, [cfg])

  // Handlers
  const updateConfig = newCfg => {
    fetch('/api/autotester/config', {
      method: 'PUT', credentials: 'include', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(newCfg),
    })
      .then(r => {
        if (!r.ok) throw new Error('Failed to update config');
        setCfg(newCfg);
      })
      .catch(err => alert(`Failed to update config: ${err.message}`));
  }

  const runTest = key => {
    fetch(`/api/autotester/run/${key}`, { method: 'POST', credentials: 'include' })
  }

  const calibrate = key => {
    fetch(`/api/autotester/calibrate/${key}`, { method: 'POST', credentials: 'include' })
  }

  const savePumpVolume = () => {
    const vol = parseFloat(pumpVolume)
    if (isNaN(vol)) return

    const factor = vol / 50.0
    // POST new calibration factor to backend → Arduino
    fetch('/api/autotester/calibrate/pump', {
      method: 'POST',
      credentials: 'include',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ value: factor }),
    })
      .then(r => {
        if (!r.ok) throw new Error('Calibration failed')
        return fetch('/api/autotester/config', { credentials: 'include' })
      })
      .then(r => r.json())
      .then(setCfg)
      .catch(console.error)
      .finally(() => setPumpCalibrating(false))
  }

  // Save a parameter calibration factor to backend (and Arduino)
  const saveParamCalibration = () => {
    const val = parseFloat(paramValue)
    if (isNaN(val)) return
    fetch(`/api/autotester/calibrate/${paramCalibrating}/start`, {
      method: 'POST',
      credentials: 'include',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ value: val }),
    })
      .then(r => {
        if (!r.ok) throw new Error('Calibration failed')
        return fetch('/api/autotester/config', { credentials: 'include' })
      })
      .then(r => r.json())
      .then(setCfg)
      .catch(err => alert(`Calibration failed: ${err.message}`))
      .finally(() => setParamCalibrating(null))
  }

  return (
    <div className="p-4">
      <div className="flex justify-between items-center mb-4">
        <h2 className="text-2xl font-bold">Auto Tester</h2>
        <button
          className="bg-indigo-600 text-white px-3 py-1 rounded"
          onClick={() => setShowConfig(true)}
        >Configure AutoTester</button>
      </div>

      {showConfig && <ConfigPanel onClose={() => setShowConfig(false)} />}

      {/* Queue & Log Section */}
      <div className="mb-4 p-4 bg-gray-100 rounded shadow">
        <h3 className="font-semibold mb-2">Pending Queue</h3>
        <QueueLog
          tasks={queue}
          onCancel={param => {
            fetch(`/api/autotester/queue/${param}`, { method: 'DELETE', credentials: 'include' })
              .then(() => setQueue(q => q.filter(t => t.param !== param)))
          }}
        />
        <h3 className="mt-4 font-semibold mb-2">Activity Log</h3>
        <LogOutput logs={logs} />
      </div>

      {/* Calibration Panel removed */}

      {/* Parameter Cards */}
      <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
        {PARAMS.map(p => {
          const remKey = `reagent_remain_${p.key}`
          const useKey = `reagent_use_${p.key}`
          // Prevent duplicate enqueues
          const isQueued = queue.some(t => t.param === p.key)
          // New: only allow Run if idle and all other conditions
          const canRun =
            cfg[`enable_${p.key}`] &&
            overallStatus === 'Idle' &&
            !isQueued &&
            cfg[remKey] >= cfg[useKey] &&
            (cfg.waste_remaining + cfg[useKey] <= cfg.waste_threshold)
          return (
            <div key={p.key} className="bg-white rounded shadow p-4">
              <div className="flex justify-between items-center border-b pb-2 mb-2">
                <strong>{p.label}</strong>
                <input
                  type="checkbox"
                  checked={cfg[`enable_${p.key}`]}
                  disabled={overallStatus !== 'Idle'}
                  onChange={e => updateConfig({ ...cfg, [`enable_${p.key}`]: e.target.checked })}
                  className="form-checkbox h-5 w-5 text-blue-600"
                />
              </div>

              <div className="mb-2 text-sm text-gray-700">
                Reagent: {cfg[remKey]} mL &bull; Waste: {cfg.waste_remaining} mL
              </div>

              <div className="mb-2">
                <label className="font-medium mr-2">Schedule:</label>
                <span className="mr-2">{cfg[`schedule_${p.key}`] || 'Disabled'}</span>
                <button className="text-sm text-blue-700 underline" onClick={() => setScheduleEditParam(p.key)}>
                  {cfg[`schedule_${p.key}`] ? 'Edit' : 'Set'}
                </button>
              </div>

              <div className="mb-2">
                <button
                  className="bg-blue-600 text-white text-sm px-3 py-1 rounded mr-2 disabled:opacity-50"
                  disabled={!canRun}
                  onClick={() => runTest(p.key)}
                >Run Now</button>
              </div>

              {/* per‑card status removed */}

              {Array.isArray(data[p.key]) && data[p.key].length > 0 ? (
                <Chart data={data[p.key]} label={p.label} unit={p.unit} height={200} />
              ) : (
                <div className="text-sm text-gray-500">Perform a test to see data</div>
              )}
            </div>
          )
        })}
      </div>

      {scheduleEditParam && (
        <ScheduleForm
          param={scheduleEditParam}
          currentRule={cfg[`schedule_${scheduleEditParam}`]}
          onSave={rule => { updateConfig({ ...cfg, [`schedule_${scheduleEditParam}`]: rule }); setScheduleEditParam(null) }}
          onClose={() => setScheduleEditParam(null)}
        />
      )}

      {paramCalibrating && (
        <div className="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50">
          <div className="bg-white rounded shadow-lg w-80 p-4">
            <h3 className="text-lg font-semibold mb-2">
              Calibrate {paramCalibrating.toUpperCase()}
            </h3>
            <p className="text-sm mb-2">
              After running a test with a known standard, enter its true value:
            </p>
            <input
              type="number"
              step="any"
              placeholder="e.g. 250.0"
              className="w-full border rounded px-2 py-1 mb-4"
              value={paramValue}
              onChange={e => setParamValue(e.target.value)}
            />
            <div className="flex justify-end space-x-2">
              <button
                onClick={() => setParamCalibrating(null)}
                className="px-3 py-1 bg-gray-200 rounded hover:bg-gray-300"
              >
                Cancel
              </button>
              <button
                onClick={saveParamCalibration}
                className="px-3 py-1 bg-blue-600 text-white rounded hover:bg-blue-700"
              >
                Save
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
