// front-end/src/autotester/chart.jsx
import React from 'react'
import {
  ResponsiveContainer,
  Tooltip,
  YAxis,
  XAxis,
  LineChart,
  Line,
} from 'recharts'

export default function Chart({ data = [], label, unit, height }) {
  // 1) Convert your backend’s ts (seconds) → ms and sort
  const points = data
    .map(d => ({ ts: d.ts * 1000, value: d.value }))
    .sort((a, b) => a.ts - b.ts)

  if (!points.length) {
    return <div className="w-full text-center text-gray-500">No data yet</div>
  }

  const nowMs = Date.now()
  const weekAgoMs = nowMs - 7 * 24 * 60 * 60 * 1000
  const earliestTsMs = points[0].ts

  // 2) Compute dynamic domain start:
  //    - if your data covers <7 days, start at your earliest point
  //    - once data span ≥7 days, start at exactly 7 days ago
  const domainStart = (nowMs - earliestTsMs < 7 * 24 * 60 * 60 * 1000)
    ? earliestTsMs
    : weekAgoMs

  return (
    <div className="w-full">
      <span className="block text-lg font-semibold mb-2">{label}</span>
      <ResponsiveContainer width="100%" height={height}>
        <LineChart data={points}>
          <Line
            dataKey="value"
            isAnimationActive={false}
            dot={false}
            className="stroke-current text-blue-600"
          />
          <XAxis
            dataKey="ts"
            type="number"
            scale="time"
            domain={[domainStart, nowMs]}
            // label each tick as Day or time
            tickFormatter={ts =>
              new Date(ts).toLocaleDateString(undefined, { month: 'short', day: 'numeric' })
            }
            className="text-xs text-gray-600"
            tickCount={7}
          />
          <Tooltip
            labelFormatter={ts => new Date(ts).toLocaleString()}
            formatter={val => [val, unit]}
            contentStyle={{ fontSize: '0.75rem' }}
          />
          <YAxis className="text-xs text-gray-600" />
        </LineChart>
      </ResponsiveContainer>
    </div>
  )
}
