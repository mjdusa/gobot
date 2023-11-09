package gpio

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func initTestEasyDriverWithStubbedAdaptor() (*EasyDriver, *gpioTestAdaptor) {
	const anglePerStep = 0.5 // use non int step angle to check int math

	a := newGpioTestAdaptor()
	d := NewEasyDriver(a, anglePerStep, "1", "2", "3", "4")
	return d, a
}

func TestNewEasyDriver(t *testing.T) {
	// arrange
	const anglePerStep = 0.5 // use non int step angle to check int math

	a := newGpioTestAdaptor()
	// act
	d := NewEasyDriver(a, anglePerStep, "1", "2", "3", "4")
	// assert
	assert.IsType(t, &EasyDriver{}, d)
	assert.True(t, strings.HasPrefix(d.name, "EasyDriver"))
	assert.Equal(t, a, d.connection)
	assert.NoError(t, d.afterStart())
	assert.NoError(t, d.beforeHalt())
	assert.NotNil(t, d.Commander)
	assert.NotNil(t, d.mutex)
	assert.Equal(t, "1", d.stepPin)
	assert.Equal(t, "2", d.dirPin)
	assert.Equal(t, "3", d.enPin)
	assert.Equal(t, "4", d.sleepPin)
	assert.Equal(t, float32(anglePerStep), d.anglePerStep)
	assert.Equal(t, uint(14), d.speedRpm)
	assert.Equal(t, "forward", d.direction)
	assert.Equal(t, 0, d.stepNum)
	assert.Equal(t, false, d.disabled)
	assert.Equal(t, false, d.sleeping)
	assert.Nil(t, d.stopAsynchRunFunc)
}

func TestEasyDriverMoveDeg_IsMoving(t *testing.T) {
	tests := map[string]struct {
		inputDeg               int
		simulateDisabled       bool
		simulateAlreadyRunning bool
		simulateWriteErr       bool
		wantWrites             int
		wantSteps              int
		wantMoving             bool
		wantErr                string
	}{
		"move_one": {
			inputDeg:   1,
			wantWrites: 4,
			wantSteps:  2,
			wantMoving: false,
		},
		"move_more": {
			inputDeg:   20,
			wantWrites: 80,
			wantSteps:  40,
			wantMoving: false,
		},
		"error_disabled": {
			simulateDisabled: true,
			wantMoving:       false,
			wantErr:          "is disabled",
		},
		"error_already_running": {
			simulateAlreadyRunning: true,
			wantMoving:             true,
			wantErr:                "already running or moving",
		},
		"error_write": {
			inputDeg:         1,
			simulateWriteErr: true,
			wantWrites:       0,
			wantMoving:       false,
			wantErr:          "write error",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// arrange
			d, a := initTestEasyDriverWithStubbedAdaptor()
			defer func() {
				// for cleanup dangling channels
				if d.stopAsynchRunFunc != nil {
					err := d.stopAsynchRunFunc(true)
					assert.NoError(t, err)
				}
			}()
			// arrange: different behavior
			d.disabled = tc.simulateDisabled
			if tc.simulateAlreadyRunning {
				d.stopAsynchRunFunc = func(bool) error { return nil }
			}
			// arrange: writes
			a.written = nil // reset writes of Start()
			a.simulateWriteError = tc.simulateWriteErr
			// act
			err := d.MoveDeg(tc.inputDeg)
			// assert
			if tc.wantErr != "" {
				assert.ErrorContains(t, err, tc.wantErr)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tc.wantSteps, d.stepNum)
			assert.Equal(t, tc.wantWrites, len(a.written))
			assert.Equal(t, tc.wantMoving, d.IsMoving())
		})
	}
}

func TestEasyDriverRun_IsMoving(t *testing.T) {
	tests := map[string]struct {
		simulateDisabled       bool
		simulateAlreadyRunning bool
		simulateWriteErr       bool
		wantMoving             bool
		wantErr                string
	}{
		"run": {
			wantMoving: true,
		},
		"error_disabled": {
			simulateDisabled: true,
			wantMoving:       false,
			wantErr:          "is disabled",
		},
		"write_error_skipped": {
			simulateWriteErr: true,
			wantMoving:       true,
		},
		"error_already_running": {
			simulateAlreadyRunning: true,
			wantMoving:             true,
			wantErr:                "already running or moving",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// arrange
			d, a := initTestEasyDriverWithStubbedAdaptor()
			d.skipStepErrors = true
			d.disabled = tc.simulateDisabled
			if tc.simulateAlreadyRunning {
				d.stopAsynchRunFunc = func(bool) error { return nil }
			}
			simWriteErr := tc.simulateWriteErr // to prevent data race in write function (go-called)
			a.digitalWriteFunc = func(string, byte) error {
				if simWriteErr {
					simWriteErr = false // to prevent to much output
					return fmt.Errorf("write error")
				}
				return nil
			}
			// act
			err := d.Run()
			// assert
			if tc.wantErr != "" {
				assert.ErrorContains(t, err, tc.wantErr)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tc.wantMoving, d.IsMoving())
		})
	}
}

func TestEasyDriverStop_IsMoving(t *testing.T) {
	// arrange
	d, _ := initTestEasyDriverWithStubbedAdaptor()
	require.NoError(t, d.Run())
	require.True(t, d.IsMoving())
	// act
	err := d.Stop()
	// assert
	assert.NoError(t, err)
	assert.False(t, d.IsMoving())
}

func TestEasyDriverHalt_IsMoving(t *testing.T) {
	// arrange
	d, _ := initTestEasyDriverWithStubbedAdaptor()
	require.NoError(t, d.Run())
	require.True(t, d.IsMoving())
	// act
	err := d.Halt()
	// assert
	assert.NoError(t, err)
	assert.False(t, d.IsMoving())
}

func TestEasyDriverSetDirection(t *testing.T) {
	const anglePerStep = 0.5 // use non int step angle to check int math

	tests := map[string]struct {
		input            string
		dirPin           string
		simulateWriteErr bool
		wantVal          string
		wantWritten      byte
		wantErr          string
	}{
		"forward": {
			input:       "forward",
			dirPin:      "10",
			wantWritten: 0,
			wantVal:     "forward",
		},
		"backward": {
			input:       "backward",
			dirPin:      "11",
			wantWritten: 1,
			wantVal:     "backward",
		},
		"unknown": {
			input:       "unknown",
			dirPin:      "12",
			wantWritten: 0xFF,
			wantVal:     "forward",
			wantErr:     "Invalid direction 'unknown'",
		},
		"error_no_pin": {
			input:       "forward",
			dirPin:      "",
			wantWritten: 0xFF,
			wantVal:     "forward",
			wantErr:     "dirPin is not set",
		},
		"error_write": {
			input:            "backward",
			dirPin:           "13",
			simulateWriteErr: true,
			wantWritten:      0xFF,
			wantVal:          "forward",
			wantErr:          "write error",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// arrange
			a := newGpioTestAdaptor()
			d := NewEasyDriver(a, anglePerStep, "1", tc.dirPin, "3", "4")
			a.written = nil // reset writes of Start()
			a.simulateWriteError = tc.simulateWriteErr
			require.Equal(t, "forward", d.direction)
			// act
			err := d.SetDirection(tc.input)
			// assert
			if tc.wantErr != "" {
				assert.ErrorContains(t, err, tc.wantErr)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.dirPin, a.written[0].pin)
				assert.Equal(t, tc.wantWritten, a.written[0].val)
			}
			assert.Equal(t, tc.wantVal, d.direction)
		})
	}
}

func TestEasyDriverMaxSpeed(t *testing.T) {
	const delayForMaxSpeed = 1428 * time.Microsecond // 1/700Hz

	tests := map[string]struct {
		anglePerStep float32
		want         uint
	}{
		"maxspeed_for_20spr": {
			anglePerStep: 360.0 / 20.0,
			want:         2100,
		},
		"maxspeed_for_36spr": {
			anglePerStep: 360.0 / 36.0,
			want:         1166,
		},
		"maxspeed_for_50spr": {
			anglePerStep: 360.0 / 50.0,
			want:         840,
		},
		"maxspeed_for_100spr": {
			anglePerStep: 360.0 / 100.0,
			want:         420,
		},
		"maxspeed_for_400spr": {
			anglePerStep: 360.0 / 400.0,
			want:         105,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// arrange
			d, _ := initTestEasyDriverWithStubbedAdaptor()
			d.anglePerStep = tc.anglePerStep
			d.stepsPerRev = 360.0 / tc.anglePerStep
			// act
			got := d.MaxSpeed()
			d.speedRpm = got
			got2 := d.getDelayPerStep()
			// assert
			assert.Equal(t, tc.want, got)
			assert.Equal(t, delayForMaxSpeed.Microseconds()/10, got2.Microseconds()/10)
		})
	}
}

func TestEasyDriverSetSpeed(t *testing.T) {
	const (
		anglePerStep = 10
		maxRpm       = 1166
	)

	tests := map[string]struct {
		input   uint
		want    uint
		wantErr string
	}{
		"below_minimum": {
			input:   0,
			want:    0,
			wantErr: "RPM (0) cannot be a zero or negative value",
		},
		"minimum": {
			input: 1,
			want:  1,
		},
		"maximum": {
			input: maxRpm,
			want:  maxRpm,
		},
		"above_maximum": {
			input:   maxRpm + 1,
			want:    maxRpm,
			wantErr: "cannot be greater then maximal value 1166",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// arrange
			d, _ := initTestEasyDriverWithStubbedAdaptor()
			d.speedRpm = 0
			d.anglePerStep = anglePerStep
			d.stepsPerRev = 360.0 / anglePerStep
			// act
			err := d.SetSpeed(tc.input)
			// assert
			if tc.wantErr != "" {
				assert.ErrorContains(t, err, tc.wantErr)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tc.want, d.speedRpm)
		})
	}
}

func TestEasyDriver_onePinStepping(t *testing.T) {
	tests := map[string]struct {
		countCallsForth  int
		countCallsBack   int
		simulateWriteErr bool
		wantSteps        int
		wantWritten      []gpioTestWritten
		wantErr          string
	}{
		"single": {
			countCallsForth: 1,
			wantSteps:       1,
			wantWritten: []gpioTestWritten{
				{pin: "1", val: 0x00},
				{pin: "1", val: 0x01},
			},
		},
		"many": {
			countCallsForth: 4,
			wantSteps:       4,
			wantWritten: []gpioTestWritten{
				{pin: "1", val: 0x0},
				{pin: "1", val: 0x1},
				{pin: "1", val: 0x0},
				{pin: "1", val: 0x1},
				{pin: "1", val: 0x0},
				{pin: "1", val: 0x1},
				{pin: "1", val: 0x0},
				{pin: "1", val: 0x1},
			},
		},
		"forth_and_back": {
			countCallsForth: 5,
			countCallsBack:  3,
			wantSteps:       2,
			wantWritten: []gpioTestWritten{
				{pin: "1", val: 0x0},
				{pin: "1", val: 0x1},
				{pin: "1", val: 0x0},
				{pin: "1", val: 0x1},
				{pin: "1", val: 0x0},
				{pin: "1", val: 0x1},
				{pin: "1", val: 0x0},
				{pin: "1", val: 0x1},
				{pin: "1", val: 0x0},
				{pin: "1", val: 0x1},
				{pin: "1", val: 0x0},
				{pin: "1", val: 0x1},
				{pin: "1", val: 0x0},
				{pin: "1", val: 0x1},
				{pin: "1", val: 0x0},
				{pin: "1", val: 0x1},
			},
		},
		"reverse": {
			countCallsBack: 3,
			wantSteps:      -3,
			wantWritten: []gpioTestWritten{
				{pin: "1", val: 0x0},
				{pin: "1", val: 0x1},
				{pin: "1", val: 0x0},
				{pin: "1", val: 0x1},
				{pin: "1", val: 0x0},
				{pin: "1", val: 0x1},
			},
		},
		"error_write": {
			simulateWriteErr: true,
			countCallsBack:   2,
			wantErr:          "write error",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// arrange
			d, a := initTestEasyDriverWithStubbedAdaptor()
			a.written = nil // reset writes of Start()
			a.simulateWriteError = tc.simulateWriteErr
			var errs []string
			// act
			for i := 0; i < tc.countCallsForth; i++ {
				if err := d.onePinStepping(); err != nil {
					errs = append(errs, err.Error())
				}
			}
			d.direction = "backward"
			for i := 0; i < tc.countCallsBack; i++ {
				if err := d.onePinStepping(); err != nil {
					errs = append(errs, err.Error())
				}
			}
			// assert
			if tc.wantErr != "" {
				assert.Contains(t, strings.Join(errs, ","), tc.wantErr)
			} else {
				assert.Nil(t, errs)
			}
			assert.Equal(t, tc.wantSteps, d.stepNum)
			assert.Equal(t, tc.wantSteps, d.CurrentStep())
			assert.Equal(t, tc.wantWritten, a.written)
		})
	}
}

func TestEasyDriverEnable_IsEnabled(t *testing.T) {
	const anglePerStep = 0.5 // use non int step angle to check int math

	tests := map[string]struct {
		enPin            string
		simulateWriteErr bool
		wantWrites       int
		wantEnabled      bool
		wantErr          string
	}{
		"basic": {
			enPin:       "10",
			wantWrites:  1,
			wantEnabled: true,
		},
		"with_run": {
			enPin:       "11",
			wantWrites:  1,
			wantEnabled: true,
		},
		"error_no_pin": {
			enPin:       "",
			wantWrites:  0,
			wantEnabled: true, // is enabled by default
			wantErr:     "enPin is not set",
		},
		"error_write": {
			enPin:            "12",
			simulateWriteErr: true,
			wantWrites:       0,
			wantEnabled:      false,
			wantErr:          "write error",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// arrange
			a := newGpioTestAdaptor()
			d := NewEasyDriver(a, anglePerStep, "1", "2", tc.enPin, "4")
			a.written = nil // reset writes of Start()
			a.simulateWriteError = tc.simulateWriteErr
			d.disabled = true
			require.False(t, d.IsEnabled())
			// act
			err := d.Enable()
			// assert
			assert.Equal(t, tc.wantWrites, len(a.written))
			if tc.wantErr != "" {
				assert.ErrorContains(t, err, tc.wantErr)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.enPin, a.written[0].pin)
				assert.Equal(t, byte(0), a.written[0].val) // enable pin is active low
			}
			assert.Equal(t, tc.wantEnabled, d.IsEnabled())
		})
	}
}

func TestEasyDriverDisable_IsEnabled(t *testing.T) {
	const anglePerStep = 0.5 // use non int step angle to check int math

	tests := map[string]struct {
		enPin            string
		runBefore        bool
		simulateWriteErr bool
		wantWrites       int
		wantEnabled      bool
		wantErr          string
	}{
		"basic": {
			enPin:       "10",
			wantWrites:  1,
			wantEnabled: false,
		},
		"with_run": {
			enPin:       "10",
			runBefore:   true,
			wantWrites:  1,
			wantEnabled: false,
		},
		"error_no_pin": {
			enPin:       "",
			wantWrites:  0,
			wantEnabled: true, // is enabled by default
			wantErr:     "enPin is not set",
		},
		"error_write": {
			enPin:            "12",
			simulateWriteErr: true,
			wantWrites:       1,
			wantEnabled:      true,
			wantErr:          "write error",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// arrange
			a := newGpioTestAdaptor()
			d := NewEasyDriver(a, anglePerStep, "1", "2", tc.enPin, "4")
			var numCallsWrite int
			var writtenPin string
			writtenValue := byte(0xFF)
			a.digitalWriteFunc = func(pin string, val byte) error {
				if pin == d.stepPin {
					// we do not consider call of step()
					return nil
				}
				numCallsWrite++
				writtenPin = pin
				writtenValue = val
				if tc.simulateWriteErr {
					return fmt.Errorf("write error")
				}
				return nil
			}
			if tc.runBefore {
				require.NoError(t, d.Run())
				require.True(t, d.IsMoving())
				time.Sleep(time.Millisecond)
			}
			d.disabled = false
			require.True(t, d.IsEnabled())
			// act
			err := d.Disable()
			// assert
			if tc.wantErr != "" {
				assert.ErrorContains(t, err, tc.wantErr)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, byte(1), writtenValue) // enable pin is active low
			}
			assert.Equal(t, tc.wantEnabled, d.IsEnabled())
			assert.False(t, d.IsMoving())
			assert.Equal(t, tc.wantWrites, numCallsWrite)
			assert.Equal(t, tc.enPin, writtenPin)
		})
	}
}

func TestEasyDriverSleep_IsSleeping(t *testing.T) {
	const anglePerStep = 0.5 // use non int step angle to check int math

	tests := map[string]struct {
		sleepPin         string
		runBefore        bool
		simulateWriteErr bool
		wantWrites       int
		wantSleep        bool
		wantErr          string
	}{
		"basic": {
			sleepPin:   "10",
			wantWrites: 1,
			wantSleep:  true,
		},
		"with_run": {
			sleepPin:   "11",
			runBefore:  true,
			wantWrites: 1,
			wantSleep:  true,
		},
		"error_no_pin": {
			sleepPin:   "",
			wantSleep:  false,
			wantWrites: 0,
			wantErr:    "sleepPin is not set",
		},
		"error_write": {
			sleepPin:         "12",
			simulateWriteErr: true,
			wantWrites:       1,
			wantSleep:        false,
			wantErr:          "write error",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// arrange
			a := newGpioTestAdaptor()
			d := NewEasyDriver(a, anglePerStep, "1", "2", "3", tc.sleepPin)
			d.sleeping = false
			require.False(t, d.IsSleeping())
			// arrange: writes
			var numCallsWrite int
			var writtenPin string
			writtenValue := byte(0xFF)
			a.digitalWriteFunc = func(pin string, val byte) error {
				if pin == d.stepPin {
					// we do not consider call of step()
					return nil
				}
				numCallsWrite++
				writtenPin = pin
				writtenValue = val
				if tc.simulateWriteErr {
					return fmt.Errorf("write error")
				}
				return nil
			}
			if tc.runBefore {
				require.NoError(t, d.Run())
			}
			// act
			err := d.Sleep()
			// assert
			if tc.wantErr != "" {
				assert.ErrorContains(t, err, tc.wantErr)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, byte(0), writtenValue) // sleep pin is active low
			}
			assert.Equal(t, tc.wantSleep, d.IsSleeping())
			assert.Equal(t, tc.wantWrites, numCallsWrite)
			assert.Equal(t, tc.sleepPin, writtenPin)
		})
	}
}

func TestEasyDriverWake_IsSleeping(t *testing.T) {
	const anglePerStep = 0.5 // use non int step angle to check int math

	tests := map[string]struct {
		sleepPin         string
		simulateWriteErr bool
		wantWrites       int
		wantSleep        bool
		wantErr          string
	}{
		"basic": {
			sleepPin:   "10",
			wantWrites: 1,
			wantSleep:  false,
		},
		"error_no_pin": {
			sleepPin:   "",
			wantWrites: 0,
			wantSleep:  true,
			wantErr:    "sleepPin is not set",
		},
		"error_write": {
			sleepPin:         "12",
			simulateWriteErr: true,
			wantWrites:       1,
			wantSleep:        true,
			wantErr:          "write error",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// arrange
			a := newGpioTestAdaptor()
			d := NewEasyDriver(a, anglePerStep, "1", "2", "3", tc.sleepPin)
			d.sleeping = true
			require.True(t, d.IsSleeping())
			// arrange: writes
			var numCallsWrite int
			var writtenPin string
			writtenValue := byte(0xFF)
			a.digitalWriteFunc = func(pin string, val byte) error {
				if pin == d.stepPin {
					// we do not consider call of step()
					return nil
				}
				numCallsWrite++
				writtenPin = pin
				writtenValue = val
				if tc.simulateWriteErr {
					return fmt.Errorf("write error")
				}
				return nil
			}
			// act
			err := d.Wake()
			// assert
			if tc.wantErr != "" {
				assert.ErrorContains(t, err, tc.wantErr)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, byte(1), writtenValue) // sleep pin is active low
			}
			assert.Equal(t, tc.wantSleep, d.IsSleeping())
			assert.Equal(t, tc.wantWrites, numCallsWrite)
			assert.Equal(t, tc.sleepPin, writtenPin)
		})
	}
}
