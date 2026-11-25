package t3code

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/darkyeg/spinup/internal/atomicfile"
)

// SetupSetting identifies one setting managed by setup. Value is a JSON literal.
type SetupSetting struct {
	Path, Field, Value string
	existingOnly       bool
}

func (t T3) linkSetting() SetupSetting {
	return SetupSetting{Path: t.settingsPath(), Field: "linkedEnvironmentsEnabled", Value: "true"}
}

func (t T3) networkSetting() SetupSetting {
	return SetupSetting{Path: t.desktopSettings(), Field: "serverExposureMode", Value: `"network-accessible"`, existingOnly: true}
}

func (t T3) SettingsDrift() ([]SetupSetting, error) {
	var changes []SetupSetting
	for _, setting := range []SetupSetting{t.linkSetting(), t.networkSetting()} {
		data, err := setting.planned()
		if err != nil {
			return changes, err
		}
		if data != nil {
			changes = append(changes, setting)
		}
	}
	return changes, nil
}

func (s SetupSetting) planned() ([]byte, error) {
	settings := map[string]json.RawMessage{}
	raw, err := os.ReadFile(s.Path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		if s.existingOnly {
			return nil, nil
		}
	case err != nil:
		return nil, err
	default:
		if json.Unmarshal(raw, &settings) != nil || settings == nil {
			return nil, fmt.Errorf("%s must contain a JSON object", s.Path)
		}
	}
	if string(settings[s.Field]) == s.Value {
		return nil, nil
	}
	settings[s.Field] = json.RawMessage(s.Value)
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func (s SetupSetting) apply() (bool, error) {
	data, err := s.planned()
	if data == nil || err != nil {
		return false, err
	}
	return true, atomicfile.Write(s.Path, data, 0o600)
}
