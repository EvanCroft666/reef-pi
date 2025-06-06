import React from 'react'

export default function QueueLog({ tasks = [], onCancel }) {
  if (tasks.length === 0) {
    return (
      <div className="text-sm text-gray-600">
        No pending tests
      </div>
    )
  }

  return (
    <ul className="space-y-1">
      {tasks.map(task => {
        const name = task.param.toUpperCase()
        let label
        if (task.param.startsWith('flush_')) {
          // Flush tasks show as “Flush”
          const real = task.param.replace('flush_', '').toUpperCase()
          label = `${real} Flush`
        } else if (task.param === 'pump') {
          label = 'Pump Calibration'
        } else if (task.code >= 0x22) {
          // codes 0x22–0x26 are parameter calibrations
          label = `${name} Calibration`
        } else {
          label = `${name} Test`
        }

        return (
          <li
            key={task.id}
            className="flex justify-between items-center text-sm bg-white rounded px-3 py-1 shadow-sm"
          >
            <span>{label}</span>
            <button
              onClick={() => onCancel(task.param)}
              className="text-red-600 hover:underline"
            >
              Cancel
            </button>
          </li>
        )
      })}
    </ul>
  )
}
