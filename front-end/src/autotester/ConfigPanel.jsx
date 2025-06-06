import React, { useState, useEffect } from 'react';

export default function ConfigPanel({ onClose }) {
  const [cfg, setCfg] = useState(null);
  const [saving, setSaving] = useState(false);
  // Pump calibration phases: 'idle' | 'pumping' | 'waiting'
  const [pumpPhase, setPumpPhase] = useState('idle');
  const [measuredVolume, setMeasuredVolume] = useState('');
  // Per-parameter calibration phases & measured volumes
  const [paramPhases, setParamPhases] = useState({
    ca: 'idle', alk: 'idle', mg: 'idle', no3: 'idle', po4: 'idle'
  });
  // Track the user‐entered measured mL for each parameter
  const [measuredVolumes, setMeasuredVolumes] = useState({
    ca: '', alk: '', mg: '', no3: '', po4: ''
  });

  useEffect(() => {
    fetch('/api/autotester/config')
      .then(res => {
        if (!res.ok) throw new Error(`HTTP ${res.status} ${res.statusText}`);
        return res.json();
      })
      .then(setCfg)
      .catch(err => {
        alert(`Error loading config: ${err}`);
        // Fallback default config
        setCfg({
          id: 'default',
          enable: false,
          i2c_addr: 0x10,
          enable_ca: false,
          enable_alk: false,
          enable_mg: false,
          enable_no3: false,
          enable_po4: false,
          schedule_ca: '',
          schedule_alk: '',
          schedule_mg: '',
          schedule_no3: '',
          schedule_po4: '',
          pump_calibration: 1.0,
          calibration_ca: 1.0,
          calibration_alk: 1.0,
          calibration_mg: 1.0,
          calibration_no3: 1.0,
          calibration_po4: 1.0,
          reagent_use_ca: 2.0,
          reagent_use_alk: 2.0,
          reagent_use_mg: 2.0,
          reagent_use_no3: 2.0,
          reagent_use_po4: 2.0,
          reagent_start_ca: 500.0,
          reagent_start_alk: 500.0,
          reagent_start_mg: 500.0,
          reagent_start_no3: 500.0,
          reagent_start_po4: 500.0,
          reagent_remain_ca: 500.0,
          reagent_remain_alk: 500.0,
          reagent_remain_mg: 500.0,
          reagent_remain_no3: 500.0,
          reagent_remain_po4: 500.0,
          waste_threshold: 1000.0,
          waste_remaining: 0.0,
        });
      });
  }, []);

  const updateField = (field, value) => {
    setCfg(prev => ({ ...prev, [field]: value }));
  };

  const saveConfig = () => {
    setSaving(true);
    fetch('/api/autotester/config', {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(cfg),
    })
      .then(res => {
        if (!res.ok) throw new Error(res.statusText);
        alert('Configuration saved');
      })
      .catch(err => alert(`Save failed: ${err.message}`))
      .finally(() => setSaving(false));
  };

  const fillReagent = (param) => {
    fetch(`/api/autotester/fill/${param}`, { method: 'POST' })
      .then(res => {
        if (!res.ok) throw new Error(res.statusText);
        alert(`${param.toUpperCase()} reagent refilled`);
        // Refresh config to get updated remaining volumes
        return fetch('/api/autotester/config');
      })
      .then(res => res.json())
      .then(setCfg)
      .catch(err => alert(`Fill failed: ${err.message}`));
  };

  // NEW: start pump calibration
  const calibratePump = () => {
    setPumpPhase('pumping');
    fetch('/api/autotester/calibrate/pump/start', {
      method: 'POST',
      credentials: 'include',
    })
      .then(res => {
        if (!res.ok) throw new Error(res.statusText);
        alert('Pump calibration started');
      })
      .catch(err => {
        alert(`Pump calibrate failed: ${err.message}`);
        setPumpPhase('idle');
      });
  }

  // Poll I²C status to detect end of pumping
  useEffect(() => {
    if (pumpPhase !== 'pumping') return;
    const iv = setInterval(() => {
      fetch('/api/autotester/status/pump', { credentials: 'include' })
        .then(r => r.json())
        .then(js => {
          if (js.status === 0) {
            clearInterval(iv);
            setPumpPhase('waiting');
          }
        })
        .catch(() => {});
    }, 1000);
    return () => clearInterval(iv);
  }, [pumpPhase]);

  // Submit the user‑measured volume
  const submitMeasuredVolume = () => {
    const val = parseFloat(measuredVolume);
    if (isNaN(val) || val <= 0) {
      alert('Enter a valid volume');
      return;
    }
    fetch('/api/autotester/calibrate/pump', {
      method: 'POST',
      credentials: 'include',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ value: val }),
    })
      .then(res => {
        if (!res.ok) throw new Error(res.statusText);
        return fetch('/api/autotester/config', { credentials: 'include' });
      })
      .then(r => r.json())
      .then(newCfg => {
        setCfg(newCfg);
        alert('Pump calibration complete');
      })
      .catch(err => alert(`Calibration failed: ${err.message}`))
      .finally(() => {
        setPumpPhase('idle');
        setMeasuredVolume('');
      });
  };

  // NEW: start a parameter calibration
  const startParamCalibration = key => {
    // Validate user-entered known value
    const val = parseFloat(measuredVolumes[key]);
    if (isNaN(val) || val <= 0) {
      alert('Enter a valid known value');
      return;
    }
    // Trigger pumping phase
    setParamPhases(prev => ({ ...prev, [key]: 'pumping' }));
    // Send known value to backend to enqueue calibration task
    fetch(`/api/autotester/calibrate/${key}/start`, {
      method: 'POST',
      credentials: 'include',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ value: val }),
    })
      .then(res => {
        if (!res.ok) throw new Error(res.statusText);
        alert(`${key.toUpperCase()} calibration started`);
      })
      .catch(err => {
        alert(`Calibration start failed: ${err.message}`);
        setParamPhases(prev => ({ ...prev, [key]: 'idle' }));
      });
  };

  // Poll status to detect end of pumping for any parameter calibration
  useEffect(() => {
    const active = Object.entries(paramPhases).find(([, phase]) => phase === 'pumping');
    if (!active) return;
    const [key] = active;
    const iv = setInterval(() => {
      fetch(`/api/autotester/status/${key}`, { credentials: 'include' })
        .then(r => r.json())
        .then(js => {
          if (js.status === 0) {
            clearInterval(iv);
            // Fetch new config to get updated calibration factor
            fetch('/api/autotester/config')
              .then(r => r.json())
              .then(newCfg => {
                setCfg(newCfg);
                alert(`${key.toUpperCase()} calibration complete`);
              });
            // Reset state
            setParamPhases(prev => ({ ...prev, [key]: 'idle' }));
            setMeasuredVolumes(prev => ({ ...prev, [key]: '' }));
          }
        })
        .catch(() => {});
    }, 1000);
    return () => clearInterval(iv);
  }, [paramPhases]);

  // Submit the user‑measured value for a parameter
  const submitParamMeasured = key => {
    const val = parseFloat(measuredVolumes[key]);
    if (isNaN(val) || val <= 0) {
      alert('Enter a valid measured volume');
      return;
    }
    fetch(`/api/autotester/calibrate/${key}`, {
      method: 'POST',
      credentials: 'include',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ value: val }),
    })
      .then(res => {
        if (!res.ok) throw new Error(res.statusText);
        return fetch('/api/autotester/config', { credentials: 'include' });
      })
      .then(r => r.json())
      .then(newCfg => {
        setCfg(newCfg);
        alert(`${key.toUpperCase()} calibration complete`);
      })
      .catch(err => alert(`Calibration failed: ${err.message}`))
      .finally(() => {
        setParamPhases(prev => ({ ...prev, [key]: 'idle' }));
        setMeasuredVolumes(prev => ({ ...prev, [key]: '' }));
      });
  };

  if (!cfg) return <div>Loading configuration...</div>;

  return (
    <div style={{ padding: '1rem' }}>
      <h2>AutoTester Configuration</h2>

      <div style={{ margin: '1rem 0' }}>
        <h3>Pump Calibration</h3>
        <p style={{ color: '#666', marginBottom: '0.5rem' }}>
          The pump calibration factor represents the number of steps required to dispense 1 mL of liquid.
          This value is automatically calculated during calibration.
        </p>
        <div style={{ display: 'flex', alignItems: 'center', gap: '1rem' }}>
          <div>
            <label>Calibration Factor (steps/mL): </label>
            <input
              type="number"
              step="0.01"
              value={cfg.pump_calibration}
              disabled
              style={{ fontWeight: 'bold' }}
            />
          </div>
          {pumpPhase === 'idle' && (
            <button
              onClick={calibratePump}
              className="bg-indigo-600 text-white px-3 py-1 rounded"
            >
              Calibrate Pump
            </button>
          )}
          {pumpPhase === 'pumping' && (
            <button
              disabled
              className="bg-gray-600 text-white px-3 py-1 rounded"
            >
              Pumping...
            </button>
          )}
          {pumpPhase === 'waiting' && (
            <div>
              <p>Waiting for dispensed amount (mL):</p>
              <input
                type="number"
                step="any"
                value={measuredVolume}
                onChange={e => setMeasuredVolume(e.target.value)}
              />
              <button
                onClick={submitMeasuredVolume}
                className="bg-blue-600 text-white px-3 py-1 rounded ml-2"
              >
                Submit
              </button>
            </div>
          )}
        </div>
      </div>

      <div style={{ margin: '2rem 0' }}>
        <h3>Parameter Calibrations</h3>
        <p style={{ color: '#666', marginBottom: '1rem' }}>
          Each parameter has its own calibration factor that adjusts the test results to match known standards.
          These values are automatically calculated during calibration.
        </p>
        {['ca','alk','mg','no3','po4'].map(key => (
          <div key={key} style={{ border: '1px solid #ccc', padding: '1rem', marginBottom: '1rem' }}>
            <h3>{key.toUpperCase()}</h3>
            <p>Remaining: {cfg[`reagent_remain_${key}`]} mL &bull; Waste: {cfg.waste_remaining} mL</p>

            <div style={{ marginBottom: '1rem' }}>
              <label>Calibration Factor: </label>
              <input
                type="number"
                step="0.01"
                value={cfg[`calibration_${key}`]}
                disabled
                style={{ fontWeight: 'bold' }}
              />
            </div>

            {/* === Per-Parameter Calibration Controls === */}
            <div style={{ marginTop: '0.5rem' }}>
              {paramPhases[key] === 'idle' && (
                <div className="flex items-center mt-2">
                  <input
                    type="number"
                    step="any"
                    placeholder={`Known ${key.toUpperCase()} value`}
                    className="border rounded px-2 py-1 mr-2"
                    value={measuredVolumes[key] || ''}
                    onChange={e => {
                      const value = e.target.value;
                      setMeasuredVolumes(prev => ({ ...prev, [key]: value }));
                    }}
                  />
                  <button
                    onClick={() => startParamCalibration(key)}
                    className="bg-indigo-600 text-white px-3 py-1 rounded"
                    disabled={!measuredVolumes[key]}
                  >
                    Start Calibration
                  </button>
                </div>
              )}
              {paramPhases[key] === 'pumping' && (
                <button
                  disabled
                  className="bg-gray-600 text-white px-3 py-1 rounded"
                >
                  Running Test...
                </button>
              )}
              {/* no waiting state: UI resets automatically when calibration completes */}
            </div>

            <div style={{ marginTop: '1rem' }}>
              <label>Use per Test (mL): </label>
              <input
                type="number"
                step="0.1"
                value={cfg[`reagent_use_${key}`]}
                onChange={e => updateField(`reagent_use_${key}`, parseFloat(e.target.value))}
              />
            </div>

            <div style={{ marginTop: '0.5rem' }}>
              <label>Starting Volume (mL): </label>
              <input
                type="number"
                step="0.1"
                value={cfg[`reagent_start_${key}`]}
                onChange={e => updateField(`reagent_start_${key}`, parseFloat(e.target.value))}
              />
            </div>

            <button 
              onClick={() => fillReagent(key)}
              className="bg-green-600 text-white px-3 py-1 rounded mt-2"
            >
              Fill {key.toUpperCase()}
            </button>
          </div>
        ))}
      </div>

      <div style={{ marginTop: '1rem' }}>
        <label>Waste Threshold (mL): </label>
        <input
          type="number"
          step="1"
          value={cfg.waste_threshold}
          onChange={e => updateField('waste_threshold', parseFloat(e.target.value))}
        />
      </div>

      <div style={{ marginTop: '1rem' }}>
        <button onClick={onClose}>Cancel</button>
        <button onClick={saveConfig} disabled={saving}>{saving ? 'Saving...' : 'Save'}</button>
      </div>
    </div>
  );
}
