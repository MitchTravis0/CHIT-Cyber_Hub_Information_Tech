//go:build windows

package startup

import (
	"os"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// runKeys are the six places Windows launches something from at sign-in. The
// 32-bit Run key is the one Task Manager's Startup tab does not show, which is
// exactly where a surprising amount of junk hides.
var runKeys = []struct {
	root   registry.Key
	path   string
	source string
}{
	{registry.CURRENT_USER, `SOFTWARE\Microsoft\Windows\CurrentVersion\Run`, "HKCU Run"},
	{registry.CURRENT_USER, `SOFTWARE\Microsoft\Windows\CurrentVersion\RunOnce`, "HKCU RunOnce"},
	{registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows\CurrentVersion\Run`, "HKLM Run"},
	{registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows\CurrentVersion\RunOnce`, "HKLM RunOnce"},
	{registry.LOCAL_MACHINE, `SOFTWARE\WOW6432Node\Microsoft\Windows\CurrentVersion\Run`, "HKLM Run (32-bit)"},
	{registry.LOCAL_MACHINE, `SOFTWARE\WOW6432Node\Microsoft\Windows\CurrentVersion\RunOnce`, "HKLM RunOnce (32-bit)"},
}

// collect reads the Run keys, the two Startup folders and the service list.
// Every source is optional: one that fails contributes nothing and, where it
// matters, adds a sentence to the note.
func collect(r *Report) {
	// Reading a file's version resource needs calls that are not in
	// golang.org/x/sys, and inventing a publisher is worse than saying nothing.
	r.markUnsupported(FieldPublisher)

	for _, key := range runKeys {
		collectRunKey(r, key.root, key.path, key.source)
	}
	collectStartupFolder(r, os.Getenv("APPDATA"), "Startup folder")
	collectStartupFolder(r, os.Getenv("PROGRAMDATA"), "Startup folder (all users)")
	collectServices(r)
}

func collectRunKey(r *Report, root registry.Key, path, source string) {
	key, err := registry.OpenKey(root, path, registry.QUERY_VALUE)
	if err != nil {
		// A Run key that does not exist is normal, not a failure.
		return
	}
	defer key.Close()

	names, err := key.ReadValueNames(0)
	if err != nil {
		return
	}
	for _, name := range names {
		command, _, err := key.GetStringValue(name)
		if err != nil {
			continue
		}
		r.Items = append(r.Items, Item{
			Name:      name,
			Kind:      KindStartup,
			Source:    source,
			Command:   command,
			StartMode: StartAutomatic,
			Enabled:   true,
		})
	}
}

func collectStartupFolder(r *Report, base, source string) {
	if base == "" {
		return
	}
	dir := filepath.Join(base, "Microsoft", "Windows", "Start Menu", "Programs", "Startup")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, file := range entries {
		if file.IsDir() || strings.EqualFold(file.Name(), "desktop.ini") {
			continue
		}
		full := filepath.Join(dir, file.Name())
		r.Items = append(r.Items, Item{
			Name:      strings.TrimSuffix(file.Name(), filepath.Ext(file.Name())),
			Kind:      KindStartup,
			Source:    source,
			Command:   full,
			StartMode: StartAutomatic,
			Enabled:   true,
		})
	}
}

func collectServices(r *Report) {
	key, err := registry.OpenKey(registry.LOCAL_MACHINE,
		`SYSTEM\CurrentControlSet\Services`, registry.ENUMERATE_SUB_KEYS)
	if err != nil {
		r.markUnsupported(FieldServices)
		r.addNote("This computer would not let CHIT read the service list, so only the startup entries are shown.")
		return
	}
	defer key.Close()

	names, err := key.ReadSubKeyNames(0)
	if err != nil {
		r.markUnsupported(FieldServices)
		r.addNote("This computer would not let CHIT read the service list, so only the startup entries are shown.")
		return
	}

	states := serviceStates(r)

	for _, name := range names {
		sub, err := registry.OpenKey(registry.LOCAL_MACHINE,
			`SYSTEM\CurrentControlSet\Services\`+name, registry.QUERY_VALUE)
		if err != nil {
			continue
		}

		serviceType, _, typeErr := sub.GetIntegerValue("Type")
		if typeErr != nil || !windowsIsService(serviceType) {
			sub.Close()
			continue
		}

		display, _, err := sub.GetStringValue("DisplayName")
		if err != nil || strings.TrimSpace(display) == "" {
			display = name
		}
		image, _, _ := sub.GetStringValue("ImagePath")
		start, _, _ := sub.GetIntegerValue("Start")
		sub.Close()

		mode := windowsStartMode(start)
		item := Item{
			Name:      windowsDeviceDesc(display),
			Kind:      KindService,
			Source:    "Services",
			Command:   image,
			StartMode: mode,
			Enabled:   mode != StartDisabled && mode != "",
		}
		if state, known := states[strings.ToLower(name)]; known {
			item.State = state
		}
		r.Items = append(r.Items, item)
	}
}

// serviceStates asks the service control manager which services are running.
// SC_MANAGER_CONNECT and SC_MANAGER_ENUMERATE_SERVICE are granted to ordinary
// users, unlike the full access mgr.Connect asks for, so this works without
// admin rights. When it fails the configured list is still complete and only
// the "Now" column is empty.
func serviceStates(r *Report) map[string]string {
	handle, err := windows.OpenSCManager(nil, nil,
		windows.SC_MANAGER_CONNECT|windows.SC_MANAGER_ENUMERATE_SERVICE)
	if err != nil {
		r.markUnsupported(FieldState)
		r.addNote("This computer would not tell CHIT which services are running right now, so the \"Now\" column is empty. The list of what is configured is still complete.")
		return nil
	}
	defer windows.CloseServiceHandle(handle)

	var needed, returned, resume uint32
	err = windows.EnumServicesStatusEx(handle, windows.SC_ENUM_PROCESS_INFO,
		windows.SERVICE_WIN32, windows.SERVICE_STATE_ALL,
		nil, 0, &needed, &returned, &resume, nil)
	if err != nil && err != windows.ERROR_MORE_DATA {
		r.markUnsupported(FieldState)
		r.addNote("This computer would not tell CHIT which services are running right now, so the \"Now\" column is empty. The list of what is configured is still complete.")
		return nil
	}

	buf := make([]byte, needed)
	resume = 0
	if err := windows.EnumServicesStatusEx(handle, windows.SC_ENUM_PROCESS_INFO,
		windows.SERVICE_WIN32, windows.SERVICE_STATE_ALL,
		&buf[0], uint32(len(buf)), &needed, &returned, &resume, nil); err != nil {
		r.markUnsupported(FieldState)
		r.addNote("This computer would not tell CHIT which services are running right now, so the \"Now\" column is empty. The list of what is configured is still complete.")
		return nil
	}

	out := make(map[string]string, returned)
	services := unsafe.Slice((*windows.ENUM_SERVICE_STATUS_PROCESS)(unsafe.Pointer(&buf[0])), int(returned))
	for _, service := range services {
		name := windows.UTF16PtrToString(service.ServiceName)
		state := StateStopped
		if service.ServiceStatusProcess.CurrentState == windows.SERVICE_RUNNING ||
			service.ServiceStatusProcess.CurrentState == windows.SERVICE_START_PENDING {
			state = StateRunning
		}
		out[strings.ToLower(name)] = state
	}
	return out
}
