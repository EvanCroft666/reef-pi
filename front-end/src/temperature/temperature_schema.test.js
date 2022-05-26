import TemperatureSchema from './temperature_schema'

describe('Validation', () => {
  let tc = {}

  beforeEach(() => {
    tc = {
      name: 'name',
      sensor: 'sensor',
      enable: true,
      fahrenheit: true,
      period: 60,
      alerts: false,
      notify: { enable: false },
      control: '',
      heater: '',
      cooler: '',
      hysteresis: 0,
      chart: {ymin: 0, ymax: 100, color: '#000'}
    }
  })

  it('should require min and max alert when alert is enabled', () => {
    tc.alerts = true
    expect.assertions(1)
    return TemperatureSchema.isValid(tc).then(
      valid => expect(valid).toBe(false)
    )
  })

  it('should require min and max alert when alert is enabled', () => {
    tc.alerts = true
    tc.minAlert = 77
    tc.maxAlert = 81
    expect.assertions(1)
    return TemperatureSchema.isValid(tc).then(
      valid => expect(valid).toBe(true)
    )
  })

  it('should be valid when heater and cooler to be different and have min/max', () => {
    tc.control = 'macro'
    tc.heater = '2'
    tc.cooler = '4'
    tc.min = 77
    tc.max = 80
    expect.assertions(1)
    return TemperatureSchema.isValid(tc).then(
      valid => expect(valid).toBe(true)
    )
  })

  it('should be valid when cooler < heater and either is disabled', () => {
    tc.control = 'macro'
    tc.heater = '2'
    tc.cooler = '4'
    tc.min = 77
    tc.max = 50

    expect.assertions(2)

    const tc1 = Object.assign(tc, { heater: '' })
    const valid1 = TemperatureSchema.isValid(tc1).then(
      valid => expect(valid).toBe(true)
    )

    const tc2 = Object.assign(tc, { cooler: '' })
    const valid2 = TemperatureSchema.isValid(tc2).then(
      valid => expect(valid).toBe(true)
    )

    return Promise.all([valid1, valid2])
  })

  it('should be invalid when heater and cooler are the same', () => {
    tc.control = 'macro'
    tc.heater = '2'
    tc.cooler = '2'
    tc.min = 77
    tc.max = 80
    expect.assertions(1)
    return TemperatureSchema.isValid(tc).then(
      valid => expect(valid).toBe(false)
    )
  })

  it('should require min when a heater is selected', () => {
    tc.control = 'macro'
    tc.heater = '2'
    expect.assertions(1)
    return TemperatureSchema.isValid(tc).then(
      valid => expect(valid).toBe(false)
    )
  })

  it('should require max when a chiller is selected', () => {
    tc.control = 'macro'
    tc.cooler = '2'
    expect.assertions(1)
    return TemperatureSchema.isValid(tc).then(
      valid => expect(valid).toBe(false)
    )
  })

  it('should be invalid when thresholds for heater and cooler overlap', () => {
    tc.control = 'macro'
    tc.heater = '2'
    tc.cooler = '4'
    tc.min = 77
    tc.max = 77
    expect.assertions(1)
    return TemperatureSchema.isValid(tc).then(
      valid => expect(valid).toBe(false)
    )
  })
})
