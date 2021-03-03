package dht

import (
	"machine"
	"time"
)

type device struct {
	pin machine.Pin

	measurements DeviceType
	initialized  bool

	temperature int16
	humidity    uint16
}

func (t *device) ReadMeasurements() error {
	_, _, err := t.Measurements()
	return err
}

func (t *device) Temperature() (int16, error) {
	if !t.initialized {
		return 0, UninitializedDataError
	}
	return t.temperature, nil
}

func (t *device) TemperatureFloat(scale TemperatureScale) (float32, error) {
	if !t.initialized {
		return 0, UninitializedDataError
	}
	return scale.convertToFloat(t.temperature), nil
}

func (t *device) Humidity() (uint16, error) {
	if !t.initialized {
		return 0, UninitializedDataError
	}
	return t.humidity, nil
}

func (t *device) HumidityFloat() (float32, error) {
	if !t.initialized {
		return 0, UninitializedDataError
	}
	return float32(t.humidity) / 10., nil
}

func initiateCommunication(p machine.Pin) {
	// Send low signal to the device
	p.Configure(machine.PinConfig{Mode: machine.PinOutput})
	p.Low()
	time.Sleep(startingLow)
	// Set pin to high and wait for reply
	p.High()
	p.Configure(machine.PinConfig{Mode: machine.PinInput})
}

func (t *device) Measurements() (temperature int16, humidity uint16, err error) {
	// initial waiting
	state := powerUp(t.pin)
	defer t.pin.Set(state)
	err = t.read()
	if err != nil {
		temperature = t.temperature
		humidity = t.humidity
		t.initialized = true
	}
	return
}

func (t *device) read() error {
	// initialize loop variables
	bufferData := [5]byte{}
	buf := bufferData[:]
	signalsData := [80]counter{}
	signals := signalsData[:]

	initiateCommunication(t.pin)
	err := waitForDataTransmission(t.pin)
	if err != nil {
		return err
	}
	t.receiveSignals(signals)

	err = t.extractData(signals[:], buf)
	if err != nil {
		return err
	}
	if !isValid(buf[:]) {
		return ChecksumError
	}

	t.temperature, t.humidity = t.measurements.extractData(buf)
	return nil
}

func (t *device) receiveSignals(result []counter) {
	i := uint8(0)
	machine.UART1.Interrupt.Disable()
	defer machine.UART1.Interrupt.Enable()
	for ; i < 40; i++ {
		result[i*2] = expectChange(t.pin, false)
		result[i*2+1] = expectChange(t.pin, true)
	}
}
func (t *device) extractData(signals []counter, buf []uint8) error {
	for i := uint8(0); i < 40; i++ {
		lowCycle := signals[i*2]
		highCycle := signals[i*2+1]
		if lowCycle == timeout || highCycle == timeout {
			return NoDataError
		}
		byteN := i >> 3
		buf[byteN] <<= 1
		if highCycle > lowCycle {
			buf[byteN] |= 1
		}
	}
	return nil
}

func waitForDataTransmission(p machine.Pin) error {
	// wait for thermometer to pull down
	if expectChange(p, true) == timeout {
		return NoSignalError
	}
	//wait for thermometer to pull up
	if expectChange(p, false) == timeout {
		return NoSignalError
	}
	// wait for thermometer to pull down and start sending the data
	if expectChange(p, true) == timeout {
		return NoSignalError
	}
	return nil
}

type Device interface {
	ReadMeasurements() error
	Measurements() (temperature int16, humidity uint16, err error)
	Temperature() (int16, error)
	TemperatureFloat(scale TemperatureScale) (float32, error)
	Humidity() (uint16, error)
	HumidityFloat() (float32, error)
}

func New(pin machine.Pin, deviceType DeviceType) Device {
	return &managedDevice{
		t: device{
			pin:          pin,
			measurements: deviceType,
			initialized:  false,
		},
		lastUpdate: time.Time{},
		policy: UpdatePolicy{
			UpdateTime:          time.Second * 2,
			UpdateAutomatically: true,
		},
	}
}

func NewWithPolicy(pin machine.Pin, deviceType DeviceType, updatePolicy UpdatePolicy) Device {
	return &managedDevice{
		t: device{
			pin:          pin,
			measurements: deviceType,
			initialized:  false,
		},
		lastUpdate: time.Time{},
		policy:     updatePolicy,
	}
}