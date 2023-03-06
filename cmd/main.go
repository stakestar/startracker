package main

import "github.com/stakestar/startracker/cli"

var (
	AppName = "SSV Node Tracker"
	Version = "latest"
)

func main() {
	cli.Execute(AppName, Version)
}
