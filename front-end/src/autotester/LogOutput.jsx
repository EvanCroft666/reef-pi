import React from 'react'

export default function LogOutput({ logs }) {
  // Ensure logs is an array
  const arr = Array.isArray(logs) ? logs : []
  // Keep only the last 10 entries
  const recentLogs = arr.slice(-10)

  return (
    <textarea
      readOnly
      value={recentLogs.join('\n')}
      className="bg-black text-green-200 text-xs p-2 h-40 w-full font-mono overflow-y-auto resize-none"
    />
  )
}
