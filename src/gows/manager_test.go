package gows

import (
	"testing"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waCompanionReg"
)

func TestBrowserPlatformType(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected waCompanionReg.DeviceProps_PlatformType
	}{
		// Browsers keep mapping the way they always did
		{name: "chrome", input: "Chrome", expected: waCompanionReg.DeviceProps_CHROME},
		{name: "firefox", input: "Firefox", expected: waCompanionReg.DeviceProps_FIREFOX},
		{name: "ie", input: "IE", expected: waCompanionReg.DeviceProps_IE},
		{name: "opera", input: "Opera", expected: waCompanionReg.DeviceProps_OPERA},
		{name: "safari", input: "Safari", expected: waCompanionReg.DeviceProps_SAFARI},
		{name: "edge", input: "Edge", expected: waCompanionReg.DeviceProps_EDGE},

		// Non-browser platform types are now reachable as well
		{name: "desktop", input: "Desktop", expected: waCompanionReg.DeviceProps_DESKTOP},
		{name: "desktop lowercase", input: "desktop", expected: waCompanionReg.DeviceProps_DESKTOP},
		{name: "desktop uppercase", input: "DESKTOP", expected: waCompanionReg.DeviceProps_DESKTOP},
		{name: "desktop padded", input: "  Desktop  ", expected: waCompanionReg.DeviceProps_DESKTOP},
		{name: "underscored", input: "Android_Phone", expected: waCompanionReg.DeviceProps_ANDROID_PHONE},

		// Anything we don't know still falls back to UNKNOWN
		{name: "typo", input: "Dekstop", expected: waCompanionReg.DeviceProps_UNKNOWN},
		{name: "empty", input: "", expected: waCompanionReg.DeviceProps_UNKNOWN},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := browserPlatformType(tt.input)
			if actual.String() != tt.expected.String() {
				t.Errorf("browserPlatformType(%q) = %s, expected %s", tt.input, actual, tt.expected)
			}
		})
	}
}

func TestSetKeepAliveInterval(t *testing.T) {
	restoreMin, restoreMax := whatsmeow.KeepAliveIntervalMin, whatsmeow.KeepAliveIntervalMax
	t.Cleanup(func() {
		whatsmeow.KeepAliveIntervalMin, whatsmeow.KeepAliveIntervalMax = restoreMin, restoreMax
	})

	tests := []struct {
		name        string
		min         time.Duration
		max         time.Duration
		expectedMin time.Duration
		expectedMax time.Duration
	}{
		{name: "both set", min: 8 * time.Second, max: 12 * time.Second, expectedMin: 8 * time.Second, expectedMax: 12 * time.Second},
		{name: "only min above default max - max adjusted", min: 40 * time.Second, max: 0, expectedMin: 40 * time.Second, expectedMax: 50 * time.Second},
		{name: "max equal to min - max adjusted", min: 10 * time.Second, max: 10 * time.Second, expectedMin: 10 * time.Second, expectedMax: 20 * time.Second},
		{name: "max below min - max adjusted", min: 30 * time.Second, max: 10 * time.Second, expectedMin: 30 * time.Second, expectedMax: 40 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			whatsmeow.KeepAliveIntervalMin, whatsmeow.KeepAliveIntervalMax = restoreMin, restoreMax
			min, max := SetKeepAliveInterval(tt.min, tt.max)
			if min != tt.expectedMin || max != tt.expectedMax {
				t.Errorf("SetKeepAliveInterval(%s, %s) = %s/%s, expected %s/%s", tt.min, tt.max, min, max, tt.expectedMin, tt.expectedMax)
			}
			if whatsmeow.KeepAliveIntervalMin != tt.expectedMin || whatsmeow.KeepAliveIntervalMax != tt.expectedMax {
				t.Errorf("whatsmeow globals = %s/%s, expected %s/%s", whatsmeow.KeepAliveIntervalMin, whatsmeow.KeepAliveIntervalMax, tt.expectedMin, tt.expectedMax)
			}
		})
	}
}
