package hypervisor

import (
	"errors"
	"net"
)

const stateUnknown = "UNKNOWN State"
const volumeFormatUnknown = "UNKNOWN VolumeFormat"

var (
	stateToText = map[State]string{
		StateStarting:      "starting",
		StateRunning:       "running",
		StateFailedToStart: "failed to start",
		StateStopping:      "stopping",
		StateStopped:       "stopped",
		StateDestroying:    "destroying",
	}
	textToState map[string]State

	volumeFormatToText = map[VolumeFormat]string{
		VolumeFormatRaw:   "raw",
		VolumeFormatQCOW2: "qcow2",
	}
	textToVolumeFormat map[string]VolumeFormat
)

func init() {
	textToState = make(map[string]State, len(stateToText))
	for state, text := range stateToText {
		textToState[text] = state
	}
	textToVolumeFormat = make(map[string]VolumeFormat, len(volumeFormatToText))
	for format, text := range volumeFormatToText {
		textToVolumeFormat[text] = format
	}
}

func shrinkIP(netIP net.IP) net.IP {
	switch len(netIP) {
	case 4:
		return netIP
	case 16:
		if ip4 := netIP.To4(); ip4 == nil {
			return netIP
		} else {
			return ip4
		}
	default:
		return netIP
	}
}

func (address *Address) Shrink() {
	address.IpAddress = shrinkIP(address.IpAddress)
}

func (state State) MarshalText() ([]byte, error) {
	if text := state.String(); text == stateUnknown {
		return nil, errors.New(text)
	} else {
		return []byte(text), nil
	}
}

func (state State) String() string {
	if text, ok := stateToText[state]; ok {
		return text
	} else {
		return stateUnknown
	}
}

func (state *State) UnmarshalText(text []byte) error {
	txt := string(text)
	if val, ok := textToState[txt]; ok {
		*state = val
		return nil
	} else {
		return errors.New("unknown State: " + txt)
	}
}

func (subnet *Subnet) Shrink() {
	subnet.IpGateway = shrinkIP(subnet.IpGateway)
	subnet.IpMask = shrinkIP(subnet.IpMask)
	for index, ipAddr := range subnet.DomainNameServers {
		subnet.DomainNameServers[index] = shrinkIP(ipAddr)
	}
}

func (volumeFormat VolumeFormat) MarshalText() ([]byte, error) {
	if text := volumeFormat.String(); text == volumeFormatUnknown {
		return nil, errors.New(text)
	} else {
		return []byte(text), nil
	}
}

func (volumeFormat VolumeFormat) String() string {
	if text, ok := volumeFormatToText[volumeFormat]; ok {
		return text
	} else {
		return volumeFormatUnknown
	}
}

func (volumeFormat *VolumeFormat) UnmarshalText(text []byte) error {
	txt := string(text)
	if val, ok := textToVolumeFormat[txt]; ok {
		*volumeFormat = val
		return nil
	} else {
		return errors.New("unknown VolumeFormat: " + txt)
	}
}