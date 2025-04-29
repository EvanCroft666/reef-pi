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
        const isCal = task.code >= 0x21
        const label = isCal
          ? (task.param === 'pump'
              ? 'Pump Calibration'
              : `${name} Calibration`)
          : `${name} Test`

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
