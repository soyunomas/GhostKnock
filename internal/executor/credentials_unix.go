//go:build !windows

package executor

import (
	"fmt"
	"os/exec"
	"os/user"
	"strconv"
	"syscall"
)

// setCredentials aplica el UID/GID al comando.
func setCredentials(cmd *exec.Cmd, u *user.User) error {
	uid, err := strconv.ParseUint(u.Uid, 10, 32)
	if err != nil {
		return fmt.Errorf("no se pudo parsear el UID '%s': %w", u.Uid, err)
	}

	gid, err := strconv.ParseUint(u.Gid, 10, 32)
	if err != nil {
		return fmt.Errorf("no se pudo parsear el GID '%s': %w", u.Gid, err)
	}

	cmd.SysProcAttr = &syscall.SysProcAttr{}
	cmd.SysProcAttr.Credential = &syscall.Credential{Uid: uint32(uid), Gid: uint32(gid)}

	return nil
}
