package main

import (
	"os"
	"path"

	"github.com/Cloud-Foundations/Dominator/lib/json"
	"github.com/Cloud-Foundations/Dominator/lib/log"
	"github.com/Cloud-Foundations/Dominator/lib/verstr"
	"github.com/Cloud-Foundations/Dominator/proto/sub"
)

func loadConfiguration(confDir string, conf *sub.Configuration,
	logger log.Logger) {
	file, err := os.Open(confDir)
	if err != nil {
		if !os.IsNotExist(err) {
			logger.Println(err)
		}
		return
	}
	names, err := file.Readdirnames(-1)
	file.Close()
	if err != nil {
		logger.Println(err)
		return
	}
	verstr.Sort(names)
	for _, name := range names {
		filename := path.Join(confDir, name)
		if err := json.ReadFromFile(filename, conf); err != nil {
			if !os.IsNotExist(err) {
				logger.Println(err)
			}
		}
	}
}
