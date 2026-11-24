package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"

	"github.com/lff2116788383/godot-mir2/server-go/internal/version"
)

func main() {
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println(version.Full())
		return
	}

	slog.Info("mir2x server-go bootstrap",
		"product", version.Product,
		"version", version.Version,
	)
	os.Exit(0)
}
