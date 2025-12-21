//go:build windows

package executor

import (
	"fmt"
	"os/exec"
	"os/user"
)

// setCredentials es un stub para Windows.
func setCredentials(cmd *exec.Cmd, u *user.User) error {
	return fmt.Errorf("la funcionalidad 'run_as_user' no está soportada actualmente en servidores Windows")
}
