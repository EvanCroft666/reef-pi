import React, { useState } from 'react'

const PRESETS = [
  { label: 'Disabled',      value: '' },
  { label: 'Every 4 hours', value: 'FREQ=HOURLY;INTERVAL=4' },
  { label: 'Every 8 hours', value: 'FREQ=HOURLY;INTERVAL=8' },
  { label: 'Every 12 hours',value: 'FREQ=HOURLY;INTERVAL=12' },
  { label: 'Every 24 hours',value: 'FREQ=DAILY;INTERVAL=1' },
  { label: 'Custom…',        value: 'CUSTOM' },
]

export default function ScheduleForm({ param, currentRule, onSave, onClose }) {
  const [selection, setSelection] = useState(
    PRESETS.some(p => p.value === currentRule) ? currentRule : 'CUSTOM'
  )
  const [customRule, setCustomRule] = useState(
    PRESETS.some(p => p.value === currentRule) ? '' : currentRule
  )

  const handleSave = () => {
    const rule = selection === 'CUSTOM' ? customRule.trim() : selection
    onSave(rule)
  }

  return (
    <div className="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50">
      <div className="bg-white rounded shadow-lg w-80 p-4">
        <h3 className="text-lg font-semibold mb-2">
          Schedule for {param.toUpperCase()}
        </h3>
        <div className="space-y-2 mb-4">
          {PRESETS.map(preset => (
            <label key={preset.value} className="flex items-center">
              <input
                type="radio"
                name="schedule"
                value={preset.value}
                checked={selection === preset.value}
                onChange={() => setSelection(preset.value)}
                className="form-radio text-blue-600"
              />
              <span className="ml-2">{preset.label}</span>
            </label>
          ))}

          {selection === 'CUSTOM' && (
            <input
              type="text"
              className="w-full border rounded px-2 py-1 mt-2"
              placeholder="e.g. FREQ=DAILY;BYHOUR=6;BYMINUTE=30"
              value={customRule}
              onChange={e => setCustomRule(e.target.value)}
            />
          )}
        </div>
        <div className="flex justify-end space-x-2">
          <button
            onClick={onClose}
            className="px-3 py-1 bg-gray-200 rounded hover:bg-gray-300"
          >
            Cancel
          </button>
          <button
            onClick={handleSave}
            className="px-3 py-1 bg-blue-600 text-white rounded hover:bg-blue-700"
          >
            Save
          </button>
        </div>
      </div>
    </div>
  )
}
