package main

import "golang.org/x/sys/windows"

func openT3Desktop(link string) bool {
	file, err := windows.UTF16PtrFromString(link)
	return err == nil && windows.ShellExecute(0, nil, file, nil, nil, windows.SW_SHOWNORMAL) == nil
}
