import React from 'react'

export default function CalibrationPanel({
  onCalibratePump,
  pumpCalibrating,
  pumpVolume,
  setPumpVolume,
  onSavePumpVolume,
}) {
  return (
    <div className="mb-4 p-4 bg-gray-100 rounded shadow">
      <h3 className="font-semibold mb-2">Calibration</h3>

      {/* Pump Calibration */}
      {!pumpCalibrating ? (
        <button
          onClick={onCalibratePump}
          className="bg-gray-700 text-white text-sm px-3 py-1 rounded hover:bg-gray-800"
        >
          Calibrate Pump
        </button>
      ) : (
        <div className="mt-2">
          <p className="text-sm mb-1">
            Pump is running… Once it stops, measure the volume dispensed (mL) and enter it:
          </p>
          <div className="flex items-center space-x-2">
            <input
              type="number"
              step="any"
              placeholder="Volume (mL)"
              className="border rounded px-2 py-1 flex-grow"
              value={pumpVolume}
              onChange={e => setPumpVolume(e.target.value)}
            />
            <button
              onClick={onSavePumpVolume}
              className="bg-blue-600 text-white text-sm px-3 py-1 rounded hover:bg-blue-700"
            >
              Save
            </button>
          </div>
        </div>
      )}

      {/* Parameter calibration note */}
      <p className="text-sm mt-4">
        To calibrate a specific parameter (e.g. Calcium), click the “Calibrate” button 
        in that parameter’s panel above.
      </p>
    </div>
  )
}
