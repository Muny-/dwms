package main

func init() {
	// reassign package vars here to customize
	batteries = []string{"BAT0"}
	netInterfaces = []string{"wlp2s0", "enx00e04c012aa8", "tun0"}

	// FontAwesome icons
	icons[wifiIcon] = ""
	icons[wiredIcon] = ""
	icons[volumeIcon] = ""
	icons[muteIcon] = ""
	icons[batteryIcon] = ""
	icons[keyboardIcon] = ""
	icons[brightnessIcon] = "☀"
	icons[musicPlayingIcon] = "▶▶"
	icons[musicPausedIcon] = "▮▮"
	icons[vpnIcon] = "🔑"
	icons[unknownIcon] = "�"
	icons[syncingMailIcon] = "⇄ 📧"
}
