package utils

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
)

var DataPath = GetPath("../data")
var ResultsPath = GetPath("results")
var Utilities = filepath.Join(DataPath, "Polybench", "utilities")
var Files = filepath.Join(DataPath, "Polybench", "datamining")
var PolybenchC = filepath.Join(Utilities, "polybench.c")

func GetDirFiles(dataPath string) []string {
	files, err := ioutil.ReadDir(dataPath)
	if err != nil {
		log.Fatal(err)
	}
	var Directory []string
	for _, f := range files {
		Directory = append(Directory, f.Name())
	}
	return Directory
}

func GetPath(newPath string) string {
	pwd, err := os.Getwd()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	data := filepath.Join(pwd, newPath)
	return data
}
