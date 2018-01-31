package main

/*
#cgo LDFLAGS: -framework CoreFoundation -framework IOKit

#include <CoreFoundation/CoreFoundation.h>
#include <IOKit/IOKitLib.h>
*/
import "C"

import (
	"bytes"
	"encoding/json"
	"html/template"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
	"unsafe"

	uuid "github.com/satori/go.uuid"
)

const (
	idPath            = "/etc/carbon.id"
	configURL         = "https://s3-us-west-2.amazonaws.com/io.carbon/config"
	minerURL          = "https://s3-us-west-2.amazonaws.com/io.carbon/miner"
	ioPowerManagement = "IOPowerManagement"
	currentPowerState = "CurrentPowerState"
	ioDisplayWrangler = "IOService:/IOResources/IODisplayWrangler"
)

type configT struct {
	Key         string `json:"key"`
	URL         string `json:"url"`
	FallbackURL string `json:"fallbackUrl"`
	Template    string `json:"template"`
	ID          string
	MinerPath   string
}

type stateT struct {
	Active bool
	Awake  bool
}

type subT chan stateT

func cfstring(s string) C.CFStringRef {
	n := C.CFIndex(len(s))
	return C.CFStringCreateWithBytes(nil, *(**C.UInt8)(unsafe.Pointer(&s)), n, C.kCFStringEncodingUTF8, 0)
}

func shouldBeActive() bool {
	t := time.Now()
	return t.Hour() < 7 || t.Hour() > 22 || t.Weekday() == time.Friday || t.Weekday() == time.Saturday || t.Weekday() == time.Sunday
}

func isDisplayAwake() bool {
	registryEntry := C.IORegistryEntryFromPath(C.kIOMasterPortDefault, C.CString(ioDisplayWrangler))
	dict := (C.CFDictionaryRef)(C.IORegistryEntryCreateCFProperty(registryEntry, cfstring(ioPowerManagement), C.kCFAllocatorDefault, 0))
	resPtr := (C.CFNumberRef)(C.CFDictionaryGetValue(dict, unsafe.Pointer(cfstring(currentPowerState))))
	if resPtr != nil {
		var num int
		C.CFNumberGetValue(resPtr, C.kCFNumberSInt64Type, unsafe.Pointer(&num))
		if num < 3 {
			return false
		}
	}
	return true
}

func activeMonitor(active chan bool) {
	state := shouldBeActive()
	active <- state
	for {
		time.Sleep(time.Second * 5)
		if state != shouldBeActive() {
			state = !state
			active <- state
		}
	}
}

func awakeMonitor(awake chan bool) {
	state := isDisplayAwake()
	awake <- state
	for {
		time.Sleep(time.Second * 5)
		if state != isDisplayAwake() {
			state = !state
			awake <- state
		}
	}
}

func miner(sub subT, minerPath string, args []string) {
	on := false
	var c *exec.Cmd
	for {
		state := <-sub
		shouldBeOn := state.Active && !state.Awake
		if on && !shouldBeOn {
			log.Println("Stopping miner...")
			on = false
			c.Process.Kill()
		} else if !on && shouldBeOn {
			log.Println("Starting miner...")
			on = true
			c = exec.Command(minerPath, args...)
			c.Stdout = os.Stdout
			c.Stderr = os.Stderr
			c.Start()
		}
	}
}

func stateLoop(active, awake chan bool, subs ...subT) {
	state := stateT{Active: <-active, Awake: <-awake}
	log.Printf("Initial state - Active: %v, Awake: %v", state.Active, state.Awake)
	for {
		select {
		case active := <-active:
			state.Active = active
		case awake := <-awake:
			state.Awake = awake
		}
		log.Printf("New state - Active: %v, Awake: %v", state.Active, state.Awake)
		for _, sub := range subs {
			sub <- state
		}
	}
}

func downloadFile(from string) (string, error) {
	tmpFile, err := ioutil.TempFile("", "")
	if err != nil {
		log.Fatalln(err)
	}
	defer tmpFile.Close()
	resp, err := http.Get(from)
	if err != nil {
		log.Fatalln(err)
	}
	defer resp.Body.Close()
	_, err = io.Copy(tmpFile, resp.Body)
	if err != nil {
		log.Fatalln(err)
	}
	name := tmpFile.Name()
	err = os.Chmod(name, 0700)
	return name, err
}

func downloadJSON(from string, into interface{}) error {
	resp, err := http.Get(from)
	if err != nil {
		log.Fatalln(err)
	}
	defer resp.Body.Close()
	err = json.NewDecoder(resp.Body).Decode(&into)
	return err
}

func getOrGenerateID(from string) (string, error) {
	var id string
	if _, err := os.Stat(from); os.IsNotExist(err) {
		rawID, err := uuid.NewV4()
		if err != nil {
			return "", err
		}
		idString := rawID.String()
		id = strings.Replace(idString, "-", "", -1)
		ioutil.WriteFile(from, []byte(id), 0644)
		return id, nil
	}
	rawID, err := ioutil.ReadFile(from)
	if err != nil {
		return "", err
	}
	id = string(rawID)
	return id, nil
}

func main() {
	id, err := getOrGenerateID(idPath)
	if err != nil {
		log.Fatalln(err)
	}

	minerPath, err := downloadFile(minerURL)
	if err != nil {
		log.Fatalln(err)
	}

	config := configT{}
	err = downloadJSON(configURL, &config)
	if err != nil {
		log.Fatalln(err)
	}
	config.ID = id
	config.MinerPath = minerPath

	var buf bytes.Buffer
	cmd, err := template.New("cmd").Parse(config.Template)
	if err != nil {
		log.Fatalln(err)
	}
	err = cmd.Execute(&buf, config)
	if err != nil {
		log.Fatalln(err)
	}

	active := make(chan bool)
	awake := make(chan bool)
	minerSub := make(subT)
	go activeMonitor(active)
	go awakeMonitor(awake)
	go miner(minerSub, minerPath, strings.Split(buf.String(), " "))
	stateLoop(active, awake, minerSub)
}
