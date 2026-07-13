package gows

import (
	"testing"

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
