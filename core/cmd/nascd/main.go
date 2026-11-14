// Command nascd runs the Nasc core on a desktop, for developing the web UI and
// trying SSH sessions without an Android device. Open the printed URL in a browser.
package main

import (
	"crypto/rand"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"

	nasc "nasc"
)

func main() {
	dir := os.Getenv("NASC_DATA")
	if dir == "" {
		home, err := os.UserConfigDir()
		if err != nil {
			log.Fatal(err)
		}
		dir = filepath.Join(home, "nasc-dev")
	}
	key, err := devMasterKey(dir)
	if err != nil {
		log.Fatal(err)
	}
	url, err := nasc.Start(dir, key)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Nasc is running. Open:", url)
	fmt.Println("Data dir:", dir)
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	<-c
	nasc.Stop()
}

// devMasterKey keeps the vault key in a file next to the data. That is fine for development
// on a desktop; the Android app keeps it in the hardware-backed Keystore instead.
func devMasterKey(dir string) ([]byte, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, "master.key")
	if b, err := os.ReadFile(path); err == nil && len(b) == 32 {
		return b, nil
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	return key, os.WriteFile(path, key, 0o600)
}
