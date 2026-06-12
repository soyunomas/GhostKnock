package config

import (
	"errors"
	"fmt"

	"gopkg.in/yaml.v3"
)

// MaxReplayWindowSeconds limits signature retention and prevents duration
// overflow when the server converts the configured window.
const MaxReplayWindowSeconds = 3600

type securityAlias Security

func (s *Security) UnmarshalYAML(node *yaml.Node) error {
	var decoded securityAlias
	if err := node.Decode(&decoded); err != nil {
		return err
	}

	*s = Security(decoded)
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == "replay_window_seconds" {
			s.replayWindowConfigured = true
			break
		}
	}
	return nil
}

func validateReplayWindowSeconds(seconds int) error {
	if seconds <= 0 {
		return errors.New("'security.replay_window_seconds' debe ser mayor que cero")
	}
	if seconds > MaxReplayWindowSeconds {
		return fmt.Errorf(
			"'security.replay_window_seconds' no puede superar %d segundos",
			MaxReplayWindowSeconds,
		)
	}
	return nil
}
