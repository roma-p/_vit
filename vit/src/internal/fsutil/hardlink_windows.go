//go:build windows
// +build windows

package fsutil

import (
	"fmt"

	"vit/internal/types"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

func CheckHardlinkSupport() (err error) {
	// 1. Check Developer Mode
	devModeEnabled, err := isWindowsDeveloperModeEnabled()
	if err == nil && devModeEnabled {
		return nil
	}

	// 2. Check if running as admin
	isAdmin, err := isRunningAsAdmin()
	if err == nil && isAdmin {
		return nil
	}

	// 3. Test directly if we can create hardlinks
	canCreateHardlink, testErr := testHardlinkCreation()
	if canCreateHardlink {
		return nil
	}

	// if not supported -> build an appropriate message
	errMess := "Hardlinks not supported on Windows: "
	var extra []any
	if !devModeEnabled && err == nil {
		errMess += "Developer Mode is disabled"
		extra = append(extra, "reason", "DevelopperModeDisabled")
	} else if testErr != nil {
		errMess += fmt.Sprintf("failed to create test hardlink: %v", testErr)
		extra = append(extra, "reason", "TestHardlinkFailed")
	} else {
		errMess += "unknown reason\n"
		extra = append(extra, "reason", "Unknown")
	}
	messSlice := []string{errMess}
	messSlice = append(messSlice, "Enable Developer Mode in Windows Settings:")
	messSlice = append(messSlice, "  Settings > Update & Security > For developers > Developer mode")
	messSlice = append(messSlice, "  Or run as Administrator (not recommended for regular use)")

	finalErr := types.NewStandardError(
		types.ErrHardlinkNotSupported,
		messSlice,
		extra,
	)

	return finalErr
}

func isWindowsDeveloperModeEnabled() (bool, error) {
	// Registry key for Developer Mode
	key, err := registry.OpenKey(
		registry.LOCAL_MACHINE,
		`SOFTWARE\Microsoft\Windows\CurrentVersion\AppModelUnlock`,
		registry.QUERY_VALUE,
	)
	if err != nil {
		// Key doesn't exist = Developer Mode not enabled
		return false, err
	}
	defer key.Close()

	val, _, err := key.GetIntegerValue("AllowDevelopmentWithoutDevLicense")
	if err != nil {
		return false, err
	}

	// 1 = enabled, 0 = disabled
	return val == 1, nil
}

func isRunningAsAdmin() (bool, error) {
	var sid *windows.SID

	err := windows.AllocateAndInitializeSid(
		&windows.SECURITY_NT_AUTHORITY,
		2,
		windows.SECURITY_BUILTIN_DOMAIN_RID,
		windows.DOMAIN_ALIAS_RID_ADMINS,
		0, 0, 0, 0, 0, 0,
		&sid,
	)
	if err != nil {
		return false, err
	}
	defer windows.FreeSid(sid)

	token := windows.Token(0)
	member, err := token.IsMember(sid)
	if err != nil {
		return false, err
	}

	return member, nil
}
