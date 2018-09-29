/*
Copyright 2018 VMware, Inc.
SPDX-License-Identifier: BSD-2-Clause
*/

package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// OOP stub
type baseContainer struct {
	pathEnvVar      string
	unsafePaths     bool
	debugMode       bool
	fileSet         map[string]bool
	folderSet       map[string]bool
	neededFolderSet map[string]bool
}

func (container baseContainer) lookEnvForFile(file string) (string, bool) {
	for _, folder := range strings.Split(container.pathEnvVar, ":") {
		newFile := folder + "/" + file
		// check if the string in input is an existing file somewhere in PATH
		if newFileInfo, err := os.Lstat(newFile); err == nil {
			// if by chance this is a folder, ignore and continue as we are expecting a file here
			if newFileInfo.Mode()&os.ModeDir != 0 {
				continue
			}
			return newFile, true
		}
	}
	return "", false
}

func (container baseContainer) addToSetsFromPath(pathString string) {
	// hardcoded paths to ignore
	ignorePaths := []string{"/dev", "/proc", "/sys", "/tmp", "/var/lib/docker"}
	if container.unsafePaths {
		ignorePaths = []string{}
	}
	// normalize pathString
	normalizedPathString := filepath.Clean(pathString)
	// check if normalizedPathString points to a symlink
	if isSym, err := IsSymlink(normalizedPathString); isSym && err == nil {
		// check if the file belongs to something we want to ignore
		for _, prefix := range ignorePaths {
			if strings.HasPrefix(normalizedPathString, prefix+"/") {
				if container.debugMode {
					fmt.Printf("[addToSetsFromPath]: ignoring file: %s\n", normalizedPathString)
				}
				return
			}
		}
		// it's not in the ignore set, so add the symlink to fileSet
		container.fileSet[normalizedPathString] = true
		// now read where this file is pointing to
		linkPath, _ := os.Readlink(normalizedPathString)
		if !path.IsAbs(linkPath) {
			// I got a relative link, expecting it to be in the same folder
			index := strings.LastIndexByte(normalizedPathString, '/')
			val := normalizedPathString[:index] + "/" + linkPath
			linkPath = val
		}
		// we can have symlinks pointing to symlinks
		if container.debugMode {
			fmt.Printf("[addToSetsFromPath]: found symlink %s to %s\n", normalizedPathString, linkPath)
		}
		container.addToSetsFromPath(linkPath)
	} else if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}
	// we are sure pathString is not a symlink
	if val, err := IsDir(normalizedPathString); val && err == nil {
		for _, prefix := range ignorePaths {
			// if normalizedPathString begins with "/prefix/" or it is exactly "/prefix"
			if strings.HasPrefix(normalizedPathString, prefix+"/") ||
				prefix == normalizedPathString {
				if container.debugMode {
					fmt.Printf("[addToSetsFromPath]: ignoring folder %s\n", normalizedPathString)
				}
				return
			}
		}
		container.folderSet[normalizedPathString] = true
	} else if !val && err == nil {
		for _, prefix := range ignorePaths {
			if strings.HasPrefix(normalizedPathString, prefix+"/") {
				if container.debugMode {
					fmt.Printf("[addToSetsFromPath]: ignoring file %s\n", normalizedPathString)
				}
				return
			}
		}
		if container.debugMode {
			fmt.Printf("[addToSetsFromPath]: adding %s\n", normalizedPathString)
		}
		container.fileSet[normalizedPathString] = true
	} else {
		fmt.Fprintln(os.Stderr, err)
		return
	}
}

func (container baseContainer) finalize() []string {
	for folder := range container.folderSet {
		if IsFolderNeeded(folder, container.fileSet, container.folderSet) {
			container.neededFolderSet[folder] = true
		} else {
			if container.debugMode {
				fmt.Printf("[finalize]: unneded folder %s\n", folder)
			}
		}
	}
	// merge everything
	allPaths := make([]string, len(container.neededFolderSet)+len(container.fileSet), len(container.neededFolderSet)+len(container.fileSet))
	i := 0
	for key := range container.neededFolderSet {
		allPaths[i] = key
		i++
	}
	for key := range container.fileSet {
		allPaths[i] = key
		i++
	}
	return allPaths
}

func newBaseContainer(pathEnvVar string, unsafePaths bool, debugMode bool) baseContainer {
	return baseContainer{pathEnvVar, unsafePaths, debugMode, make(map[string]bool), make(map[string]bool), make(map[string]bool)}
}

func main() {
	var pathEnvVar string
	var interp string
	var ok bool
	var err error
	// read arguments
	inputFlag := flag.String("i", "", "input file")
	outputFlag := flag.String("o", "rootfs.tar", "output file")
	interpFlag := flag.String("x", "/bin/sh", "ELF executable to read INTERP section from")
	unsafeFlag := flag.Bool("u", false, "unsafe archiving, disable hardcoded checks")
	debugFlag := flag.Bool("d", false, "debug mode (verbose output)")
	flag.Parse()
	if *inputFlag == "" {
		fmt.Fprintln(os.Stderr, "input file argument is required")
		fmt.Printf("Usage of %s:\n", os.Args[0])
		flag.PrintDefaults()
		return
	}
	// print flags values if in debug mode
	if *debugFlag {
		fmt.Printf("[main]: input file set to %s\n", *inputFlag)
		fmt.Printf("[main]: output file set to %s\n", *outputFlag)
		fmt.Printf("[main]: interp exec set to %s\n", *interpFlag)
		fmt.Printf("[main]: unsafe archiving set to %t\n", *unsafeFlag)
		fmt.Printf("[main]: debug mode set to true\n")
	}
	// read first the PATH env variable
	pathEnvVar, ok = os.LookupEnv("PATH")
	if !ok {
		fmt.Fprintln(os.Stderr, "couldn't retrieve PATH environment variable")
		return
	}
	// read the interp variable
	interp, err = GetInterpFromExec(*interpFlag)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}
	// OOP, for future distro specific behavior
	container := newBaseContainer(pathEnvVar, *unsafeFlag, *debugFlag)
	// add the interp section
	container.addToSetsFromPath(interp)
	// read the input file
	inputFile, err := os.Open(*inputFlag)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}
	defer inputFile.Close()
	scanner := bufio.NewScanner(inputFile)
	for scanner.Scan() {
		pathString := scanner.Text()
		// check if pathString is not pointing to an existing file or folder
		if _, err := os.Lstat(pathString); err != nil {
			if os.IsNotExist(err) && !path.IsAbs(pathString) {
				// if not an abs path, we might have something relative (i.e. a "java" string)
				foundPath, ok := container.lookEnvForFile(pathString)
				if ok {
					container.addToSetsFromPath(foundPath)
				}
			}
			// other error in os.Lstat or this line is complete "garbage", discard
			continue
		} else {
			// err is nil, pathString points to something
			container.addToSetsFromPath(pathString)
		}
	}
	if err := scanner.Err(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}
	// finalize the container and return a slice
	allPaths := container.finalize()
	if *debugFlag {
		for _, v := range allPaths {
			fmt.Printf("[main]: adding to tar %s\n", v)
		}
	}
	// create a tarfile
	if err := WriteTar(*outputFlag, allPaths); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}
	if *debugFlag {
		fmt.Printf("[main]: packing complete!\n")
	}
}
