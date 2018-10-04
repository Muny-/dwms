package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/BurntSushi/xgb"
	"github.com/BurntSushi/xgb/xproto"
)

type itemID int

const (
	batteryItem itemID = iota
	timeItem
	audioItem
	netItem
	musicItem
	keyboardItem
)

const (
	batSysPath = "/sys/class/power_supply"
	netSysPath = "/sys/class/net"
)

type iconID int

const (
	noIcon iconID = iota
	volumeIcon
	muteIcon
	timeIcon
	wifiIcon
	wiredIcon
	batteryIcon
	chargeIcon
	dischargeIcon
	fullIcon
	unknownIcon
	keyboardIcon
	brightnessIcon
	musicPlayingIcon
	musicPausedIcon
	vpnIcon
	syncingMailIcon
)

var icons = map[iconID]string{
	noIcon:           "",
	volumeIcon:       "v:",
	muteIcon:         "m:",
	timeIcon:         "",
	wiredIcon:        "e:",
	wifiIcon:         "w:",
	batteryIcon:      "b:",
	chargeIcon:       "+",
	dischargeIcon:    "-",
	fullIcon:         "=",
	unknownIcon:      "?",
	keyboardIcon:     "k:",
	brightnessIcon:   "d:",
	musicPlayingIcon: "playing:",
	musicPausedIcon:  "paused:",
	vpnIcon:          "vpn:",
	syncingMailIcon:  "[~]m...",
}

var (
	updatePeriod = 1 * time.Second
	items        = []itemID{netItem, musicItem, keyboardItem, batteryItem, audioItem, timeItem}
	statusFormat = statusFmt

	netInterfaces = []string{"wlan0", "eth0"}
	wifiFormat    = wifiFmt
	wiredFormat   = wiredFmt
	netFormat     = netFmt
	ssidRE        = regexp.MustCompile(`SSID:\s+(.*)`)
	bitrateRE     = regexp.MustCompile(`tx bitrate:\s+(\d+)`)
	signalRE      = regexp.MustCompile(`signal:\s+(-\d+)`)

	batteries    = []string{"BAT0"}
	batteryIcons = map[string]iconID{
		"Charging": chargeIcon, "Discharging": dischargeIcon, "Full": fullIcon,
	}
	batteryDevFormat = batteryDevFmt
	batteryFormat    = batteryFmt

	audioFormat = audioFmt
	//amixerRE    = regexp.MustCompile(`\[(\d+)%]\s*\[(\w+)]`)
	amixerRE = regexp.MustCompile(`(\d+)%`)

	timeFormat = timeFmt

	showDetails = false

	isSyncingMail = false
)

func wifiStatus(dev string, isUp bool) (string, bool) {
	ssid, bitrate, signal := "", 0, 0

	out, err := exec.Command("iw", "dev", dev, "link").Output()
	if err != nil {
		return "", false
	}
	if match := ssidRE.FindSubmatch(out); len(match) >= 2 {
		ssid = string(match[1])
	}

	if showDetails {
		if match := bitrateRE.FindSubmatch(out); len(match) >= 2 {
			if br, err := strconv.Atoi(string(match[1])); err == nil {
				bitrate = br
			}
		}
		if match := signalRE.FindSubmatch(out); len(match) >= 2 {
			if sig, err := strconv.Atoi(string(match[1])); err == nil {
				signal = sig
			}
		}
		return wifiFormat(dev, ssid, bitrate, signal, isUp)
	} else {
		return wifiFormat(dev, ssid, 0, 0, isUp)
	}
}

func wiredStatus(dev string, isUp bool) (string, bool) {
	speed := 0
	if spd, err := sysfsIntVal(filepath.Join(netSysPath, dev, "speed")); err == nil {
		speed = spd
	}
	return wiredFormat(dev, speed, isUp)
}

func netDevStatus(dev string) (string, bool) {
	status, err := sysfsStringVal(filepath.Join(netSysPath, dev, "operstate"))
	isUp := true
	if err != nil || status != "up" {
		isUp = false
	}

	_, err = os.Stat(filepath.Join(netSysPath, dev, "wireless"))
	isWifi := !os.IsNotExist(err)

	if isWifi {
		return wifiStatus(dev, isUp)
	}
	return wiredStatus(dev, isUp)
}

func netStatus() string {
	var netStats []string
	for _, dev := range netInterfaces {
		devStat, ok := netDevStatus(dev)
		if ok {
			netStats = append(netStats, devStat)
		}
	}

	return netFormat(netStats)
}

func wifiFmt(dev, ssid string, bitrate, signal int, isUp bool) (string, bool) {
	if showDetails {
		return fmt.Sprintf("%s %s %dMb/s %ddBm", icons[wifiIcon], ssid, bitrate, signal), isUp
	} else {
		return fmt.Sprintf("%s %s", icons[wifiIcon], ssid), isUp
	}
}

func wiredFmt(dev string, speed int, isUp bool) (string, bool) {
	return fmt.Sprintf("%s%d", icons[wiredIcon], speed), isUp
}

func netFmt(devs []string) string {
	return strings.Join(devs, " ")
}

func keyboardStatus() string {
	kbds, err := exec.Command("kbd-layout").CombinedOutput()

	if err != nil {
		println("ouch keyboard")
	}

	ofsStr := string(kbds[0:2])

	return icons[keyboardIcon] + " " + ofsStr
}

func batteryDevStatus(bat string) string {
	pct, err := sysfsIntVal(filepath.Join(batSysPath, bat, "capacity"))
	if err != nil {
		return icons[unknownIcon]
	}

	status, err := sysfsStringVal(filepath.Join(batSysPath, bat, "status"))
	if err != nil {
		return icons[unknownIcon]
	}

	if showDetails {

		voltageInt, err := sysfsIntVal(filepath.Join(batSysPath, bat, "voltage_now"))
		if err != nil {
			return icons[unknownIcon]
		}

		currentInt, err := sysfsIntVal(filepath.Join(batSysPath, bat, "current_now"))
		if err != nil {
			return icons[unknownIcon]
		}

		voltage := float32(voltageInt) / 1000000.0
		current := float32(currentInt) / 1000000.0

		power := voltage * current

		return batteryDevFormat(pct, status, voltage, current, power)
	}

	return batteryDevFormat(pct, status, 0, 0, 0)
}

func batteryStatus() string {
	var batStats []string
	for _, bat := range batteries {
		batStats = append(batStats, batteryDevStatus(bat))
	}
	return batteryFormat(batStats)
}

func batteryDevFmt(pct int, status string, voltage float32, current float32, power float32) string {
	if showDetails {
		return fmt.Sprintf("%d%% %.1fV@%s%.1fA=%s%.1fW",
			pct, voltage, icons[batteryIcons[status]], current, icons[batteryIcons[status]], power)
	} else {
		return fmt.Sprintf("%d%%%s",
			pct, icons[batteryIcons[status]])
	}
}

func batteryFmt(bats []string) string {
	return icons[batteryIcon] + " " + strings.Join(bats, "/")
}

func audioStatus() string {
	out, err := exec.Command("amixer", "get", "Master").Output()
	if err != nil {
		return icons[unknownIcon]
	}
	match := amixerRE.FindSubmatch(out)
	if len(match) < 1 {
		return icons[unknownIcon]
	}
	isMuted := false
	return audioFormat(string(match[0]), isMuted)
}

func audioFmt(vol string, isMuted bool) string {
	icon := volumeIcon
	if isMuted {
		icon = muteIcon
	}
	return fmt.Sprintf("%s %s", icons[icon], vol)
}

func timeStatus() string {
	return timeFormat(time.Now())
}

func timeFmt(t time.Time) string {
	if showDetails {
		return t.Format("2006-01-02 3:04:05 PM")
	}

	return t.Format("01-02 3:04 PM")
}

func statusFmt(s []string) string {
	return " " + strings.Join(s, " ⋮ ")
}

func displayStatus() string {
	brightnessB, err := exec.Command("light").Output()

	if err != nil {
		panic("ouchDisp")
	}

	brightness := string(brightnessB[0 : len(brightnessB)-4])

	return fmt.Sprintf("%s %s%%", icons[brightnessIcon], brightness)
}

func syncMailStatus() string {
	return fmt.Sprintf("%s", icons[syncingMailIcon])
}

func musicStatus() string {
	playbackStatusB, err := exec.Command("qdbus", "org.mpris.MediaPlayer2.spotify", "/org/mpris/MediaPlayer2", "org.mpris.MediaPlayer2.Player.PlaybackStatus").Output()

	if err != nil {
		return icons[unknownIcon]
	}

	playbackStatus := string(playbackStatusB[0 : len(playbackStatusB)-1])

	icon := icons[musicPausedIcon]

	if playbackStatus == "Playing" {
		icon = icons[musicPlayingIcon]
	}

	if showDetails {

		metadataB, err := exec.Command("qdbus", "org.mpris.MediaPlayer2.spotify", "/org/mpris/MediaPlayer2", "org.mpris.MediaPlayer2.Player.Metadata").Output()

		if err != nil {
			panic("ouchMetadata")
		}

		metadata := string(metadataB[0:])

		metadataLines := strings.Split(metadata, "\n")

		artist := "Unknown Artist"

		songTitle := "Unknown Title"

		for _, line := range metadataLines {
			lineParts := strings.Split(line, ": ")

			switch lineParts[0] {
			case "xesam:artist":
				artist = lineParts[1]
			case "xesam:title":
				songTitle = lineParts[1]
			}
		}

		return fmt.Sprintf("%s %s - %s", icon, songTitle, artist)
	} else {
		return fmt.Sprintf("%s", icon)
	}
}

func connectionStatus(displayName string, connectionType string, interfaceName string) string {
	if connectionType == "802-11-wireless" || connectionType == "802-3-ethernet" {
		conStatus, _ := netDevStatus(interfaceName)
		return conStatus
	} else if connectionType == "vpn" {
		return fmt.Sprintf("%s %s", icons[vpnIcon], displayName)
	}

	return "?? net ??"
}

func networkManagerStatus() string {
	nmCliB, err := exec.Command("nmcli", "-g", "all", "-c", "no", "connection", "show").Output()

	if err != nil {
		panic("ouch_nmcli")
	}

	nmCli := string(nmCliB[0 : len(nmCliB)-1])

	nmCliLines := strings.Split(nmCli, "\n")

	var connections []string

	for _, line := range nmCliLines {

		line = strings.Replace(line, "\\:", "_", -1)

		lineParts := strings.Split(line, ":")

		activeStatus := lineParts[11]

		if activeStatus == "activated" {
			displayName := lineParts[0]
			connectionType := lineParts[2]
			interfaceName := lineParts[10]

			if connectionType == "802-11-wireless" || connectionType == "802-3-ethernet" || connectionType == "vpn" {
				connections = append(connections, connectionStatus(displayName, connectionType, interfaceName))
			}
		}
	}

	return strings.Join(connections, " ⋮ ")
}

func updateStatus(x *xgb.Conn, root xproto.Window) {
	var stats []string

	if isSyncingMail {
		stats = append(stats, syncMailStatus())
	}

	if showDetails {
		stats = append(stats, displayStatus())
	}

	for _, item := range items {
		switch item {
		case batteryItem:
			stats = append(stats, batteryStatus())
		case audioItem:
			stats = append(stats, audioStatus())
		case netItem:
			//stats = append(stats, netStatus())
			stats = append(stats, networkManagerStatus())
		case keyboardItem:
			stats = append(stats, keyboardStatus())
		case musicItem:
			stats = append(stats, musicStatus())
		case timeItem:
			stats = append(stats, timeStatus())
		}
	}

	status := statusFormat(stats)

	xproto.ChangeProperty(x, xproto.PropModeReplace, root, xproto.AtomWmName,
		xproto.AtomString, 8, uint32(len(status)), []byte(status))
}

func sysfsIntVal(path string) (int, error) {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return 0, err
	}
	val, err := strconv.Atoi(string(bytes.TrimSpace(data)))
	if err != nil {
		return 0, err
	}
	return val, nil
}

func sysfsStringVal(path string) (string, error) {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(bytes.TrimSpace(data)), nil
}

func keyboardEventHandler(x *xgb.Conn, root xproto.Window) {
	cmd := exec.Command("xinput", "test", "14")

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		println("ouch")
	}

	reader := bufio.NewReader(stdout)

	if err := cmd.Start(); err != nil {
		println("ouch2")
	}

	for {
		input, err := reader.ReadString('\n')
		if err != nil {
			println("ouch3")
		}

		split := strings.Split(input, " ")

		action := split[1]

		keycode := split[len(split)-2]

		if keycode == "108" {
			if action == "press" {
				showDetails = true

				independentUpdateStatus(x, root)

			} else if action == "release" {
				showDetails = false

				updateStatus(x, root)
			}
		}
	}
}

func networkEventHandler(x *xgb.Conn, root xproto.Window) {
	cmd := exec.Command("nmcli", "monitor")

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		println("ouch11")
	}

	reader := bufio.NewReader(stdout)

	if err := cmd.Start(); err != nil {
		println("ouch22")
	}

	for {
		_, err := reader.ReadString('\n')
		if err != nil {
			println("ouch33")
		}

		updateStatus(x, root)
	}
}

func spotifyEventHandler(x *xgb.Conn, root xproto.Window) {
	cmd := exec.Command("dbus-monitor", "interface=org.mpris.MediaPlayer2.spotify", "path=/org/mpris/MediaPlayer2")

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		println("ouch11")
	}

	reader := bufio.NewReader(stdout)

	if err := cmd.Start(); err != nil {
		println("ouch22")
	}

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			println("ouch33")
		}

		if line == "         string \"PlaybackStatus\"\n" {
			updateStatus(x, root)
		}
	}
}

func independentUpdateStatus(x *xgb.Conn, root xproto.Window) {

	updateStatus(x, root)

	ticker := time.NewTicker(time.Second * 1)

	go func() {
		for range ticker.C {
			updateStatus(x, root)

			if !showDetails {
				ticker.Stop()
			}
		}
	}()
}

func makeNamedPipe(x *xgb.Conn, root xproto.Window) {
	pipeFile := "/home/kevin/src/dwms/evt_pipe"

	os.Remove(pipeFile)

	err := syscall.Mkfifo(pipeFile, 0666)
	if err != nil {
		panic("ouchMkfifo")
	}

	file, err := os.OpenFile(pipeFile, os.O_CREATE, os.ModeNamedPipe)
	if err != nil {
		panic("ouchOpenFile")
	}

	reader := bufio.NewReader(file)

	for {
		line, err := reader.ReadString('\n')

		if err == nil {

			doUpdateStatus := true

			switch line {
			case "updateStatus\n":
				println("got updateStatus")
				break
			case "startMailSync\n":
				isSyncingMail = true
				break
			case "endMailSync\n":
				isSyncingMail = false
				break
			default:
				println("namedPipe err nil, line not updateStatus:", line)
				doUpdateStatus = false
				break
			}

			if doUpdateStatus {
				updateStatus(x, root)
			}
		} else {
			println("namedPipe_err:", err.Error())
			makeNamedPipe(x, root)
			break
		}

		println("namedPipe_forBtm")

	}
}

func main() {
	x, err := xgb.NewConn()
	if err != nil {
		log.Fatal(err)
	}

	root := xproto.Setup(x).DefaultScreen(x).Root

	go keyboardEventHandler(x, root)

	go networkEventHandler(x, root)

	go spotifyEventHandler(x, root)

	go makeNamedPipe(x, root)

	/*for t := time.Tick(updatePeriod); ; <-t {

		updateStatus(x, root)
	}*/

	// initial status
	updateStatus(x, root)

	for {
		timer := time.NewTimer(time.Now().Truncate(time.Minute * 1).Add(time.Minute * 1).Sub(time.Now()))

		<-timer.C

		updateStatus(x, root)
	}

	/*for {
		//timer := time.NewTimer(time.Until(time.Now()))
	}*/
}
