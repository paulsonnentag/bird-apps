//go:build !windows

// Stubs so the fetching/wallpaper logic can be developed and tested on
// non-Windows machines: `go run . --no-install` writes files to the cache dir.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

func installDir() string {
	d, _ := os.UserCacheDir()
	return filepath.Join(d, appName)
}

func install() (bool, error)                            { return false, nil }
func uninstall() error                                  { return nil }
func screenSize() (int, int)                            { return 1920, 1080 }
func setWallpaper(path string) error                    { fmt.Println("wallpaper:", path); return nil }
func playSound(path string, maxLen time.Duration) error { fmt.Println("sound:", path); return nil }
func messageBox(title, msg string)                      { fmt.Printf("[%s] %s\n", title, msg) }
