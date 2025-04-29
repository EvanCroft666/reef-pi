// front-end/src/autotester/main.jsx
import React, { useState, useEffect } from 'react'
import Chart from './chart'
import ScheduleForm from './ScheduleForm'
import QueueLog from './QueueLog'
import CalibrationPanel from './CalibrationPanel'
import LogOutput from './LogOutput'

const PARAMS = [
  { key: 'ca',  label: 'Calcium',    unit: 'ppm' },
  { key: 'alk', label: 'Alkalinity', unit: 'dKH' },
  { key: 'mg',  label: 'Magnesium',  unit: 'ppm' },
  { key: 'no3', label: 'Nitrate',    unit: 'ppm', stub: true },
  { key: 'po4', label: 'Phosphate',  unit: 'ppm', stub: true },
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
  const [stat, setStat] = useState({})
  const [data, setData] = useState({})
  const [queue, setQueue] = useState([])
  const [logs, setLogs] = useState([])
  const [scheduleEditParam, setScheduleEditParam] = useState(null)
  const [pumpCalibrating, setPumpCalibrating] = useState(false)
  const [pumpVolume, setPumpVolume] = useState('')

  // Fetch config on mount
  useEffect(() => {
    fetch('/api/autotester/config', { credentials: 'include' })
      .then(r => r.ok ? r.json() : Promise.reject())
      .then(setCfg)
      .catch(() => setCfg(DEFAULT_CFG))
  }, [])

  // Poll status, results, queue, and logs every 2 seconds
  useEffect(() => {
    const iv = setInterval(async () => {
      // Status & results
      for (const { key } of PARAMS) {
        const js = await fetch(`/api/autotester/status/${key}`, { credentials: 'include' })
                         .then(r => r.json())
        setStat(s => ({ ...s, [key]: js.status }))
        if (js.status === 0) {
          const arr = await fetch(`/api/autotester/results/${key}`, { credentials: 'include' })
                           .then(r => r.json())
          setData(d => ({ ...d, [key]: arr }))
        }
      }
      // Queue list
      fetch('/api/autotester/queue', { credentials: 'include' })
        .then(r => r.json())
        .then(setQueue)
      // Activity log
      fetch('/api/autotester/log', { credentials: 'include' })
        .then(r => r.json())
        .then(setLogs)
    }, 2000)
    return () => clearInterval(iv)
  }, [])

  // Load historical data whenever config changes
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
      method: 'PUT',
      credentials: 'include',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(newCfg),
    }).then(() => setCfg(newCfg))
  }

  const runTest = key => {
    fetch(`/api/autotester/run/${key}`, {
      method: 'POST',
      credentials: 'include',
    })
  }

  const calibrate = key => {
    fetch(`/api/autotester/calibrate/${key}`, {
      method: 'POST',
      credentials: 'include',
    })
  }

  const savePumpVolume = () => {
    const vol = parseFloat(pumpVolume)
    if (!isNaN(vol)) {
      const factor = vol / 50.0
      updateConfig({ ...cfg, pump_calibration: factor })
      setPumpCalibrating(false)
    }
  }

  return (
    <div className="p-4">
      <h2 className="text-2xl font-bold mb-4">Auto Tester</h2>

      {/* Queue & Log Section */}
      <div className="mb-4 p-4 bg-gray-100 rounded shadow">
        <h3 className="font-semibold mb-2">Pending Queue</h3>
        <QueueLog
          tasks={queue}
          onCancel={param => {
            fetch(`/api/autotester/queue/${param}`, {
              method: 'DELETE',
              credentials: 'include',
            }).then(() =>
              setQueue(q => q.filter(t => t.param !== param))
            )
          }}
        />
        <h3 className="mt-4 font-semibold mb-2">Activity Log</h3>
        <LogOutput logs={logs} />
      </div>

      {/* Calibration Panel */}
      <CalibrationPanel
        onCalibratePump={() => { calibrate('pump'); setPumpCalibrating(true) }}
        pumpCalibrating={pumpCalibrating}
        pumpVolume={pumpVolume}
        setPumpVolume={setPumpVolume}
        onSavePumpVolume={savePumpVolume}
      />

      {/* Parameter Cards */}
      <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
        {PARAMS.map(p => (
          <div key={p.key} className="bg-white rounded shadow p-4">
            {/* Header */}
            <div className="flex justify-between items-center border-b pb-2 mb-2">
              <strong>{p.label}</strong>
              <input
                type="checkbox"
                checked={cfg[`enable_${p.key}`]}
                disabled={stat[p.key] === 1}
                onChange={e => updateConfig({
                  ...cfg,
                  [`enable_${p.key}`]: e.target.checked,
                })}
                className="form-checkbox h-5 w-5 text-blue-600"
              />
            </div>

            {/* Schedule */}
            <div className="mb-2">
              <label className="font-medium mr-2">Schedule:</label>
              <span className="mr-2">
                {cfg[`schedule_${p.key}`] || 'Disabled'}
              </span>
              <button
                className="text-sm text-blue-700 underline"
                onClick={() => setScheduleEditParam(p.key)}
              >
                {cfg[`schedule_${p.key}`] ? 'Edit' : 'Set'}
              </button>
            </div>

            {/* Actions */}
            <div className="mb-2">
              <button
                className="bg-blue-600 text-white text-sm px-3 py-1 rounded mr-2 disabled:opacity-50"
                disabled={!cfg[`enable_${p.key}`] || stat[p.key] === 1 || p.stub}
                onClick={() => runTest(p.key)}
              >
                Run Now
              </button>
              <button
                className="bg-gray-600 text-white text-sm px-3 py-1 rounded mr-2 disabled:opacity-50"
                disabled={stat[p.key] === 1}
                onClick={() => { calibrate('pump'); setPumpCalibrating(true) }}
              >
                Calibrate Pump
              </button>
              <button
                className="bg-gray-600 text-white text-sm px-3 py-1 rounded disabled:opacity-50"
                disabled={stat[p.key] === 1 || p.stub}
                onClick={() => calibrate(p.key)}
              >
                Calibrate {p.label}
              </button>
            </div>

            {/* Status */}
            <div className="mb-2">
              {stat[p.key] === 1 && <span className="text-blue-600">Running...</span>}
              {stat[p.key] === 2 && <span className="text-red-600">Error</span>}
              {stat[p.key] === 0 && (
                queue.find(t => t.param === p.key)
                  ? <span className="text-blue-600">Queued</span>
                  : <span className="text-gray-600">Idle</span>
              )}
            </div>

            {/* Chart or placeholder */}
            {Array.isArray(data[p.key]) && data[p.key].length > 0 ? (
              <Chart data={data[p.key]} label={p.label} unit={p.unit} height={200} />
            ) : (
              <div className="text-sm text-gray-500">Perform a test to see data</div>
            )}
          </div>
        ))}
      </div>

      {/* ScheduleForm Modal */}
      {scheduleEditParam && (
        <ScheduleForm
          param={scheduleEditParam}
          currentRule={cfg[`schedule_${scheduleEditParam}`]}
          onSave={rule => {
            updateConfig({ ...cfg, [`schedule_${scheduleEditParam}`]: rule })
            setScheduleEditParam(null)
          }}
          onClose={() => setScheduleEditParam(null)}
        />
      )}
    </div>
  )
}
