package runtime

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/qira-dev/karyora/internal/config"
	"github.com/qira-dev/karyora/internal/domain/errs"
)

// ConfigFileName is the operator configuration file inside the config class
// (C2-05: OS user-config file, path defined by C2-06).
const ConfigFileName = "config.json"

// LoadEffectiveConfig resolves the layered configuration (C2-05): compiled
// defaults → config file → environment → explicit CLI flags. A missing file
// is normal; a malformed file, unknown keys, range violations, and policy
// conflicts all fail closed with envelope errors.
func LoadEffectiveConfig(correlationID string, configDir string, flagOverrides map[string]any, env func(string) string) (config.Config, *Failure) {
	var file map[string]any
	path := filepath.Join(configDir, ConfigFileName)
	b, err := os.ReadFile(path)
	switch {
	case err == nil:
		if err := json.Unmarshal(b, &file); err != nil {
			env2, _ := errs.New(errs.CodeValidationFailed, correlationID, map[string]any{
				"fields": []map[string]string{{"path": ConfigFileName, "reason": "INVALID_CONFIG_FILE", "source": "file"}},
			})
			return config.Config{}, &Failure{Envelope: env2, Exit: ExitConfigInvalid}
		}
	case os.IsNotExist(err):
		// defaults → env → flags only
	default:
		env2, _ := errs.New(errs.CodeStorageUnavailable, correlationID, map[string]any{"resource": "config-file"})
		return config.Config{}, &Failure{Envelope: env2, Exit: ExitStorage}
	}
	cfg, err := config.Load(correlationID, file, env, flagOverrides)
	if err != nil {
		ce, ok := err.(*config.Error)
		if !ok {
			env2, _ := errs.New(errs.CodeInternal, correlationID, nil)
			return config.Config{}, &Failure{Envelope: env2, Exit: ExitInternal}
		}
		return config.Config{}, &Failure{Envelope: ce.Envelope, Exit: ExitCodeForFailure(ce.Envelope)}
	}
	return cfg, nil
}
