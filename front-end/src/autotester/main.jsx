// front-end/src/autotester/main.jsx
import React from 'react'

export default function AutoTester() {
  const poke = async (cmd) => {
    try {
      const res = await fetch('/api/autotester/led/' + cmd, {
        method: 'POST',
        credentials: 'include',
      })
      if (!res.ok) {
        alert('Error ' + res.status)
      }
    } catch (error) {
      alert('Network error: ' + error.message)
    }
  }

  return (
    <div className="container mt-4">
      <h2>Auto Tester POC</h2>
      <button
        className="btn btn-primary mr-2"
        onClick={() => poke('on')}
      >
        Love Yourself
      </button>
      <button
        className="btn btn-secondary"
        onClick={() => poke('off')}
      >
        Kill Yourself
      </button>
    </div>
  )
}