package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

var (
	user32               = windows.NewLazySystemDLL("user32.dll")
	winmm                = windows.NewLazySystemDLL("winmm.dll")
	procSystemParamsInfo = user32.NewProc("SystemParametersInfoW")
	procGetSystemMetrics = user32.NewProc("GetSystemMetrics")
	procMessageBox       = user32.NewProc("MessageBoxW")
	procMciSendString    = winmm.NewProc("mciSendStringW")
)

const (
	runKey       = `Software\Microsoft\Windows\CurrentVersion\Run`
	uninstallKey = `Software\Microsoft\Windows\CurrentVersion\Uninstall\` + appName
)

func installDir() string {
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		base, _ = os.UserCacheDir()
	}
	return filepath.Join(base, appName)
}

func installedExe() string { return filepath.Join(installDir(), appName+".exe") }

// install copies this exe into %LOCALAPPDATA% and registers it to run at
// sign-in (per-user, no admin needed). Returns true if this run is the one
// that installed it (i.e. it was launched from somewhere else).
func install() (bool, error) {
	self, err := os.Executable()
	if err != nil {
		return false, err
	}
	target := installedExe()
	if strings.EqualFold(filepath.Clean(self), filepath.Clean(target)) {
		return false, nil // already running the installed copy
	}
	if err := os.MkdirAll(installDir(), 0o755); err != nil {
		return false, err
	}
	if err := copyFile(self, target); err != nil {
		return false, err
	}

	k, _, err := registry.CreateKey(registry.CURRENT_USER, runKey, registry.SET_VALUE)
	if err != nil {
		return false, err
	}
	k.SetStringValue(appName, `"`+target+`"`)
	k.Close()

	// Show up in Settings > Apps so it's easy to remove.
	if u, _, err := registry.CreateKey(registry.CURRENT_USER, uninstallKey, registry.SET_VALUE); err == nil {
		u.SetStringValue("DisplayName", "Bird Startup")
		u.SetStringValue("DisplayIcon", target)
		u.SetStringValue("Publisher", "bird-apps")
		u.SetStringValue("InstallLocation", installDir())
		u.SetStringValue("UninstallString", `"`+target+`" --uninstall`)
		u.SetDWordValue("NoModify", 1)
		u.SetDWordValue("NoRepair", 1)
		u.Close()
	}
	return true, nil
}

func uninstall() error {
	if k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.SET_VALUE); err == nil {
		k.DeleteValue(appName)
		k.Close()
	}
	registry.DeleteKey(registry.CURRENT_USER, uninstallKey)

	// We may be running from inside the install dir, so delete it after we exit.
	cmd := exec.Command("cmd", "/c", "ping 127.0.0.1 -n 3 >nul & rmdir /s /q \""+installDir()+"\"")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
	return cmd.Start()
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp := dst + ".tmp"
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	out.Close()
	os.Remove(dst)
	return os.Rename(tmp, dst)
}

func screenSize() (int, int) {
	w, _, _ := procGetSystemMetrics.Call(0) // SM_CXSCREEN
	h, _, _ := procGetSystemMetrics.Call(1) // SM_CYSCREEN
	if w == 0 || h == 0 {
		return 1920, 1080
	}
	return int(w), int(h)
}

func setWallpaper(path string) error {
	if k, _, err := registry.CreateKey(registry.CURRENT_USER, `Control Panel\Desktop`, registry.SET_VALUE); err == nil {
		k.SetStringValue("WallpaperStyle", "10") // Fill
		k.SetStringValue("TileWallpaper", "0")
		k.Close()
	}
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	const spiSetDeskWallpaper = 0x0014
	const spifUpdateAndBroadcast = 0x01 | 0x02
	r, _, e := procSystemParamsInfo.Call(spiSetDeskWallpaper, 0, uintptr(unsafe.Pointer(p)), spifUpdateAndBroadcast)
	if r == 0 {
		return fmt.Errorf("SystemParametersInfo: %v", e)
	}
	return nil
}

func mci(cmd string) (string, error) {
	c, _ := windows.UTF16PtrFromString(cmd)
	buf := make([]uint16, 256)
	r, _, _ := procMciSendString.Call(uintptr(unsafe.Pointer(c)), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)), 0)
	if r != 0 {
		return "", fmt.Errorf("mci %q failed: code %d", cmd, r)
	}
	return windows.UTF16ToString(buf), nil
}

// playSound plays the mp3 via the built-in Windows MCI player (no codecs or
// extra DLLs needed), up to max length, fading out over the last 2 seconds.
func playSound(path string, maxLen time.Duration) error {
	if _, err := mci(`open "` + path + `" type mpegvideo alias bird`); err != nil {
		return err
	}
	defer mci("close bird")
	mci("set bird time format milliseconds")
	dur := int(maxLen.Milliseconds())
	if s, err := mci("status bird length"); err == nil {
		if n, err := strconv.Atoi(strings.TrimSpace(s)); err == nil && n > 0 && n < dur {
			dur = n
		}
	}
	const fade = 2000
	mci("setaudio bird volume to 1000")
	if _, err := mci(fmt.Sprintf("play bird from 0 to %d", dur)); err != nil {
		return err
	}
	deadline := time.Now().Add(time.Duration(dur+5000) * time.Millisecond)
	for time.Now().Before(deadline) {
		time.Sleep(100 * time.Millisecond)
		if mode, _ := mci("status bird mode"); mode != "playing" {
			break
		}
		if s, err := mci("status bird position"); err == nil {
			if pos, err := strconv.Atoi(strings.TrimSpace(s)); err == nil && pos > dur-fade {
				vol := max(0, 1000*(dur-pos)/fade)
				mci(fmt.Sprintf("setaudio bird volume to %d", vol))
			}
		}
	}
	return nil
}

func messageBox(title, msg string) {
	t, _ := windows.UTF16PtrFromString(title)
	m, _ := windows.UTF16PtrFromString(msg)
	const mbOK, mbIconInfo = 0x0, 0x40
	procMessageBox.Call(0, uintptr(unsafe.Pointer(m)), uintptr(unsafe.Pointer(t)), mbOK|mbIconInfo)
}
